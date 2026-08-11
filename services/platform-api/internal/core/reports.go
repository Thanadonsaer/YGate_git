package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"ygate/platform-api/internal/auth"
)

const (
	ReportTypeExecutive         = "EXECUTIVE_PORTFOLIO"
	ReportTypeOperational       = "OPERATIONAL_STATUS"
	ReportTypeFaultsMaintenance = "FAULTS_MAINTENANCE"
)

var ErrReportInvalid = errors.New("invalid report request")

type ReportRequest struct {
	ReportType string
	PlantIDs   []string
	From       time.Time
	To         time.Time
}

type reportPlantRow struct {
	Plant      Plant
	EventCount int64
	AlarmCount int64
}

func normalizeReportRequest(input ReportRequest) (ReportRequest, error) {
	input.ReportType = strings.ToUpper(strings.TrimSpace(input.ReportType))
	switch input.ReportType {
	case ReportTypeExecutive, ReportTypeOperational, ReportTypeFaultsMaintenance:
	default:
		return ReportRequest{}, ErrReportInvalid
	}
	if input.From.IsZero() || input.To.IsZero() || !input.To.After(input.From) || input.To.Sub(input.From) > 366*24*time.Hour {
		return ReportRequest{}, ErrReportInvalid
	}
	cleanIDs := make([]string, 0, len(input.PlantIDs))
	seen := map[string]bool{}
	for _, id := range input.PlantIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			cleanIDs = append(cleanIDs, id)
		}
	}
	input.PlantIDs = cleanIDs
	return input, nil
}

// ExportReportXLSX produces a TOR-ready operational workbook from the same
// Plant and Event Logbook data used by the monitoring pages.
func (s *Service) ExportReportXLSX(ctx context.Context, principal auth.Principal, input ReportRequest, sourceIP *netip.Addr) ([]byte, error) {
	request, err := normalizeReportRequest(input)
	if err != nil {
		return nil, err
	}
	plants, err := s.reportPlants(ctx, principal, request.PlantIDs)
	if err != nil {
		return nil, err
	}
	rows := [][]string{{"Report type", request.ReportType}, {"From", request.From.Format(time.RFC3339)}, {"To", request.To.Format(time.RFC3339)}, {}}
	switch request.ReportType {
	case ReportTypeFaultsMaintenance:
		rows = append(rows, []string{"Plant", "Event type", "Category", "Title", "Started", "Ended", "Note"})
		for _, plant := range plants {
			entries, listErr := s.ListEventLogbook(ctx, principal, plant.Plant.ID, 500)
			if listErr != nil {
				return nil, listErr
			}
			for _, entry := range entries {
				if entry.StartsAt.Before(request.From) || !entry.StartsAt.Before(request.To) {
					continue
				}
				ended := ""
				if entry.EndsAt != nil {
					ended = entry.EndsAt.Format(time.RFC3339)
				}
				rows = append(rows, []string{plant.Plant.Code, entry.EventType, entry.Category, entry.Title, entry.StartsAt.Format(time.RFC3339), ended, entry.Note})
			}
		}
	default:
		rows = append(rows, []string{"Plant code", "Plant name", "Lifecycle", "DC kW", "AC kW", "Event logbook entries", "Alarm events"})
		for _, row := range plants {
			rows = append(rows, []string{row.Plant.Code, row.Plant.Name, row.Plant.LifecycleStatus, optionalFloat(row.Plant.InstalledDcKW), optionalFloat(row.Plant.InstalledAcKW), strconv.FormatInt(row.EventCount, 10), strconv.FormatInt(row.AlarmCount, 10)})
		}
	}
	data, err := buildXLSX(ctx, rows)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Service) reportPlants(ctx context.Context, principal auth.Principal, ids []string) ([]reportPlantRow, error) {
	var plants []Plant
	if len(ids) == 0 {
		var err error
		plants, err = s.Plants(ctx, principal)
		if err != nil {
			return nil, err
		}
	} else {
		plants = make([]Plant, 0, len(ids))
		for _, id := range ids {
			plant, err := s.Plant(ctx, principal, id)
			if err != nil {
				return nil, err
			}
			plants = append(plants, plant)
		}
	}
	result := make([]reportPlantRow, 0, len(plants))
	for _, plant := range plants {
		var eventCount, alarmCount int64
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM alarm.event_logbook WHERE plant_id=$1`, plant.ID).Scan(&eventCount); err != nil {
			return nil, fmt.Errorf("count event logbook: %w", err)
		}
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM alarm.alarm_event WHERE plant_id=$1`, plant.ID).Scan(&alarmCount); err != nil {
			return nil, fmt.Errorf("count alarm events: %w", err)
		}
		result = append(result, reportPlantRow{Plant: plant, EventCount: eventCount, AlarmCount: alarmCount})
	}
	return result, nil
}

func optionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

func buildXLSX(ctx context.Context, rows [][]string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	parts := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Report" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
	}
	for name, content := range parts {
		if err := writeZipFile(archive, name, []byte(content)); err != nil {
			return nil, err
		}
	}
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range rows {
		sheet.WriteString(`<row r="` + strconv.Itoa(rowIndex+1) + `">`)
		for colIndex, value := range row {
			cell := colName(colIndex+1) + strconv.Itoa(rowIndex+1)
			sheet.WriteString(`<c r="` + cell + `" t="inlineStr"><is><t>` + xmlEscape(value) + `</t></is></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	if err := writeZipFile(archive, "xl/worksheets/sheet1.xml", []byte(sheet.String())); err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func writeZipFile(archive *zip.Writer, name string, data []byte) error {
	file, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return err
}
func colName(n int) string {
	result := ""
	for n > 0 {
		n--
		result = string(rune('A'+n%26)) + result
		n /= 26
	}
	return result
}
func xmlEscape(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
