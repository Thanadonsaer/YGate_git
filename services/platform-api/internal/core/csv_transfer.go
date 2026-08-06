package core

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"

	"ygate/platform-api/internal/auth"
)

// csv_transfer.go: bulk export/import for Register Metadata and Plant+Device
// as flat CSV, for operators who want to prepare or archive configuration in
// a spreadsheet rather than one-by-one through the UI or a live Middleware.
// Import always upserts (matches the Register Metadata page's own PUT and
// Import from Middleware) -- it never deletes rows a CSV omits.

var registerMetadataCSVHeader = []string{
	"manufacturer", "deviceType", "model", "addressKey", "displayName", "unit", "dataType",
	"scale", "offset", "decimals", "isEnabled", "notes",
	"modbusFunctionCode", "modbusRegister", "modbusWordOrder", "modbusDataType",
}

var plantDeviceCSVHeader = []string{
	"plantCode", "plantName", "timezone", "latitude", "longitude", "installedDcKw", "installedAcKw",
	"externalId", "deviceName", "manufacturer", "deviceType", "model", "modbusHost", "modbusPort", "modbusUnitId", "isActive",
}

type CSVImportResult struct {
	DeviceModelsCreated int      `json:"deviceModelsCreated"`
	DeviceModelsReused  int      `json:"deviceModelsReused"`
	PlantsCreated       int      `json:"plantsCreated"`
	PlantsUpdated       int      `json:"plantsUpdated"`
	DevicesCreated      int      `json:"devicesCreated"`
	DevicesUpdated      int      `json:"devicesUpdated"`
	RowsUpserted        int      `json:"rowsUpserted"`
	RowsSkipped         int      `json:"rowsSkipped"`
	Errors              []string `json:"errors,omitempty"`
}

func csvColumnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, name := range header {
		idx[strings.TrimSpace(name)] = i
	}
	return idx
}

func csvCell(row []string, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func readCSVRecords(data []byte) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	var records [][]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func writeCSVRecords(header []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatFloatCSV(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
func formatIntPtrCSV(v *int32) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(int(*v))
}
func formatTextPtrCSV(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// RegisterMetadataCSVTemplate returns a header-only CSV for operators to
// fill in by hand before importing.
func RegisterMetadataCSVTemplate() ([]byte, error) {
	return writeCSVRecords(registerMetadataCSVHeader, nil)
}

// PlantDeviceCSVTemplate returns a header-only CSV for operators to fill in
// by hand before importing.
func PlantDeviceCSVTemplate() ([]byte, error) {
	return writeCSVRecords(plantDeviceCSVHeader, nil)
}

// ExportRegisterMetadataCSV writes every Register Metadata row for modelID,
// or (modelID == "") for every Device Model the principal can see. Every
// row carries its owning Device Model's manufacturer/deviceType/model so a
// re-import can resolve (or create) the right model without a separate
// models sheet.
func (s *Service) ExportRegisterMetadataCSV(ctx context.Context, principal auth.Principal, modelID string) ([]byte, error) {
	all, err := s.DeviceModels(ctx, principal)
	if err != nil {
		return nil, err
	}
	models := all
	if strings.TrimSpace(modelID) != "" {
		models = nil
		for _, m := range all {
			if m.ID == modelID {
				models = append(models, m)
				break
			}
		}
		if len(models) == 0 {
			return nil, ErrNotFound
		}
	}
	var rows [][]string
	for _, m := range models {
		metadata, err := s.DeviceModelRegisterMetadata(ctx, principal, m.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range metadata {
			rows = append(rows, []string{
				m.Manufacturer, m.DeviceType, m.Model, r.AddressKey, r.DisplayName, r.Unit, r.DataType,
				formatFloatCSV(r.Scale), formatFloatCSV(r.Offset), strconv.Itoa(int(r.Decimals)),
				strconv.FormatBool(r.IsEnabled), r.Notes,
				formatIntPtrCSV(r.ModbusFunctionCode), formatIntPtrCSV(r.ModbusRegister), formatTextPtrCSV(r.ModbusWordOrder), formatTextPtrCSV(r.ModbusDataType),
			})
		}
	}
	return writeCSVRecords(registerMetadataCSVHeader, rows)
}

// ImportRegisterMetadataCSV upserts Device Models (matched by trimmed
// manufacturer/deviceType/model, same as a Middleware config import) and
// their Register Metadata rows (upserted by addressKey, same as the
// Register Metadata page's own PUT) from csvData. Never deletes rows a CSV
// omits -- purely additive/updating, unlike a live Middleware config push.
func (s *Service) ImportRegisterMetadataCSV(ctx context.Context, principal auth.Principal, organizationID string, csvData []byte, sourceIP *netip.Addr) (CSVImportResult, error) {
	var result CSVImportResult
	orgID := strings.TrimSpace(organizationID)
	if orgID == "" {
		orgID = uuidString(principal.OrganizationID)
	}
	records, err := readCSVRecords(csvData)
	if err != nil {
		return result, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return result, nil
	}
	idx := csvColumnIndex(records[0])

	existing, err := s.DeviceModels(ctx, principal)
	if err != nil {
		return result, fmt.Errorf("list existing device models: %w", err)
	}
	type modelKeyT struct{ manufacturer, deviceType, model string }
	byKey := make(map[modelKeyT]string, len(existing))
	for _, m := range existing {
		if m.OrganizationID != orgID {
			continue
		}
		byKey[modelKeyT{m.Manufacturer, m.DeviceType, m.Model}] = m.ID
	}

	for i, row := range records[1:] {
		lineNo := i + 2
		manufacturer := csvCell(row, idx, "manufacturer")
		deviceType := csvCell(row, idx, "deviceType")
		model := csvCell(row, idx, "model")
		addressKey := csvCell(row, idx, "addressKey")
		if manufacturer == "" || deviceType == "" || model == "" || addressKey == "" {
			result.RowsSkipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: manufacturer/deviceType/model/addressKey are required", lineNo))
			continue
		}
		key := modelKeyT{manufacturer, deviceType, model}
		modelID, ok := byKey[key]
		if !ok {
			created, err := s.CreateDeviceModel(ctx, principal, DeviceModelInput{
				OrganizationID: orgID, Manufacturer: manufacturer, DeviceType: deviceType, Model: model, IsActive: true,
			}, sourceIP)
			if err != nil {
				result.RowsSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: create device model: %v", lineNo, err))
				continue
			}
			modelID = created.ID
			byKey[key] = modelID
			result.DeviceModelsCreated++
		} else {
			result.DeviceModelsReused++
		}

		scaleText := csvCell(row, idx, "scale")
		scale := 1.0
		if scaleText != "" {
			if v, err := strconv.ParseFloat(scaleText, 64); err == nil {
				scale = v
			}
		}
		offset, _ := strconv.ParseFloat(csvCell(row, idx, "offset"), 64)
		decimals, _ := strconv.Atoi(csvCell(row, idx, "decimals"))
		isEnabled := true
		if v := csvCell(row, idx, "isEnabled"); v != "" {
			isEnabled, _ = strconv.ParseBool(v)
		}
		dataType := csvCell(row, idx, "dataType")
		if dataType == "" {
			dataType = "number"
		}
		input := UpdateDeviceModelRegisterMetadataInput{
			UpdateDeviceRegisterMetadataInput: UpdateDeviceRegisterMetadataInput{
				AddressKey: addressKey, DisplayName: csvCell(row, idx, "displayName"), Unit: csvCell(row, idx, "unit"),
				DataType: dataType, Scale: scale, Offset: offset, Decimals: int32(decimals), IsEnabled: isEnabled, Notes: csvCell(row, idx, "notes"),
			},
			ModbusWordOrder: csvCell(row, idx, "modbusWordOrder"),
			ModbusDataType:  csvCell(row, idx, "modbusDataType"),
		}
		if v := csvCell(row, idx, "modbusFunctionCode"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				fc := int32(n)
				input.ModbusFunctionCode = &fc
			}
		}
		if v := csvCell(row, idx, "modbusRegister"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				reg := int32(n)
				input.ModbusRegister = &reg
			}
		}
		if _, err := s.SetDeviceModelRegisterMetadata(ctx, principal, modelID, input, sourceIP); err != nil {
			result.RowsSkipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: %v", lineNo, err))
			continue
		}
		result.RowsUpserted++
	}
	return result, nil
}

// ExportPlantDeviceCSV writes one row per Device (Plant fields repeated on
// each row) for plantID, or (plantID == "") for every Plant the principal
// can see. A Plant with no Devices still gets one row so it round-trips.
func (s *Service) ExportPlantDeviceCSV(ctx context.Context, principal auth.Principal, plantID string) ([]byte, error) {
	all, err := s.Plants(ctx, principal)
	if err != nil {
		return nil, err
	}
	plants := all
	if strings.TrimSpace(plantID) != "" {
		plants = nil
		for _, p := range all {
			if p.ID == plantID {
				plants = append(plants, p)
				break
			}
		}
		if len(plants) == 0 {
			return nil, ErrNotFound
		}
	}
	var rows [][]string
	for _, p := range plants {
		devices, err := s.Devices(ctx, principal, p.ID)
		if err != nil {
			return nil, err
		}
		plantCols := []string{
			p.Code, p.Name, p.Timezone, floatPtrCSV(p.Latitude), floatPtrCSV(p.Longitude), floatPtrCSV(p.InstalledDcKW), floatPtrCSV(p.InstalledAcKW),
		}
		if len(devices) == 0 {
			rows = append(rows, append(append([]string{}, plantCols...), "", "", "", "", "", "", "", "", strconv.FormatBool(p.IsActive)))
			continue
		}
		for _, d := range devices {
			host := ""
			if d.ModbusHost != nil {
				host = *d.ModbusHost
			}
			port := ""
			if d.ModbusPort != nil {
				port = strconv.Itoa(int(*d.ModbusPort))
			}
			rows = append(rows, append(append([]string{}, plantCols...),
				d.ExternalID, d.Name, d.Manufacturer, d.DeviceType, d.Model, host, port, strconv.Itoa(int(d.ModbusUnitID)), strconv.FormatBool(d.IsActive)))
		}
	}
	return writeCSVRecords(plantDeviceCSVHeader, rows)
}

func floatPtrCSV(v *float64) string {
	if v == nil {
		return ""
	}
	return formatFloatCSV(*v)
}

// ImportPlantDeviceCSV upserts Plants (matched by code) and their Devices
// (matched by external_id within the Plant) from csvData -- Plant fields
// are read from the first row seen for each plantCode. Device Models are
// resolved/created the same trimmed way ImportRegisterMetadataCSV does.
func (s *Service) ImportPlantDeviceCSV(ctx context.Context, principal auth.Principal, organizationID string, csvData []byte, sourceIP *netip.Addr) (CSVImportResult, error) {
	var result CSVImportResult
	orgID := strings.TrimSpace(organizationID)
	if orgID == "" {
		orgID = uuidString(principal.OrganizationID)
	}
	records, err := readCSVRecords(csvData)
	if err != nil {
		return result, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return result, nil
	}
	idx := csvColumnIndex(records[0])

	existingModels, err := s.DeviceModels(ctx, principal)
	if err != nil {
		return result, fmt.Errorf("list existing device models: %w", err)
	}
	type modelKeyT struct{ manufacturer, deviceType, model string }
	modelByKey := make(map[modelKeyT]string, len(existingModels))
	for _, m := range existingModels {
		if m.OrganizationID != orgID {
			continue
		}
		modelByKey[modelKeyT{m.Manufacturer, m.DeviceType, m.Model}] = m.ID
	}

	existingPlants, err := s.Plants(ctx, principal)
	if err != nil {
		return result, fmt.Errorf("list existing plants: %w", err)
	}
	plantByCode := make(map[string]Plant, len(existingPlants))
	for _, p := range existingPlants {
		if p.OrganizationID != orgID {
			continue
		}
		plantByCode[strings.ToUpper(p.Code)] = p
	}
	deviceByExternalID := make(map[string]map[string]string) // plantID -> externalID -> deviceID

	for i, row := range records[1:] {
		lineNo := i + 2
		code := strings.ToUpper(csvCell(row, idx, "plantCode"))
		name := csvCell(row, idx, "plantName")
		if code == "" || name == "" {
			result.RowsSkipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: plantCode/plantName are required", lineNo))
			continue
		}
		plant, ok := plantByCode[code]
		if !ok {
			created, err := s.CreatePlant(ctx, principal, CreatePlantInput{
				OrganizationID: orgID, Code: code, Name: name, Timezone: firstNonEmptyCSV(csvCell(row, idx, "timezone"), "UTC"),
				Latitude: parseFloatPtrCSV(csvCell(row, idx, "latitude")), Longitude: parseFloatPtrCSV(csvCell(row, idx, "longitude")),
				InstalledDcKW: parseFloatPtrCSV(csvCell(row, idx, "installedDcKw")), InstalledAcKW: parseFloatPtrCSV(csvCell(row, idx, "installedAcKw")),
			}, sourceIP)
			if err != nil {
				result.RowsSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: create plant %s: %v", lineNo, code, err))
				continue
			}
			plant = created
			plantByCode[code] = plant
			result.PlantsCreated++
		} else {
			updated, err := s.UpdatePlant(ctx, principal, plant.ID, UpdatePlantInput{
				Code: code, Name: name, Timezone: firstNonEmptyCSV(csvCell(row, idx, "timezone"), plant.Timezone),
				Latitude: parseFloatPtrCSV(csvCell(row, idx, "latitude")), Longitude: parseFloatPtrCSV(csvCell(row, idx, "longitude")),
				InstalledDcKW: parseFloatPtrCSV(csvCell(row, idx, "installedDcKw")), InstalledAcKW: parseFloatPtrCSV(csvCell(row, idx, "installedAcKw")),
				IsActive: true,
			}, sourceIP)
			if err != nil {
				result.RowsSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: update plant %s: %v", lineNo, code, err))
				continue
			}
			plant = updated
			plantByCode[code] = plant
			result.PlantsUpdated++
		}

		externalID := csvCell(row, idx, "externalId")
		if externalID == "" {
			continue // plant-only row, no Device to sync
		}
		manufacturer := csvCell(row, idx, "manufacturer")
		deviceType := csvCell(row, idx, "deviceType")
		model := csvCell(row, idx, "model")
		if manufacturer == "" || deviceType == "" || model == "" {
			result.RowsSkipped++
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: manufacturer/deviceType/model are required for a Device row", lineNo))
			continue
		}
		modelKey := modelKeyT{manufacturer, deviceType, model}
		modelID, ok := modelByKey[modelKey]
		if !ok {
			created, err := s.CreateDeviceModel(ctx, principal, DeviceModelInput{
				OrganizationID: orgID, Manufacturer: manufacturer, DeviceType: deviceType, Model: model, IsActive: true,
			}, sourceIP)
			if err != nil {
				result.RowsSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: create device model: %v", lineNo, err))
				continue
			}
			modelID = created.ID
			modelByKey[modelKey] = modelID
			result.DeviceModelsCreated++
		} else {
			result.DeviceModelsReused++
		}

		if deviceByExternalID[plant.ID] == nil {
			plantDevices, err := s.Devices(ctx, principal, plant.ID)
			if err != nil {
				result.RowsSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: list existing devices: %v", lineNo, err))
				continue
			}
			byExternal := make(map[string]string, len(plantDevices))
			for _, d := range plantDevices {
				byExternal[d.ExternalID] = d.ID
			}
			deviceByExternalID[plant.ID] = byExternal
		}

		deviceName := firstNonEmptyCSV(csvCell(row, idx, "deviceName"), externalID)
		host := csvCell(row, idx, "modbusHost")
		var port *int32
		if v := csvCell(row, idx, "modbusPort"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				p := int32(n)
				port = &p
			}
		}
		unitID := int32(1)
		if v := csvCell(row, idx, "modbusUnitId"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				unitID = int32(n)
			}
		}

		if deviceID, exists := deviceByExternalID[plant.ID][externalID]; exists {
			if _, err := s.UpdateDevice(ctx, principal, plant.ID, deviceID, UpdateDeviceInput{
				Name: deviceName, DeviceModelID: modelID, ModbusHost: host, ModbusPort: port, ModbusUnitID: unitID, IsActive: true,
			}, sourceIP); err != nil {
				result.RowsSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: update device %s: %v", lineNo, externalID, err))
				continue
			}
			result.DevicesUpdated++
		} else {
			created, err := s.CreateDevice(ctx, principal, plant.ID, CreateDeviceInput{
				ExternalID: externalID, Name: deviceName, DeviceModelID: modelID, ModbusHost: host, ModbusPort: port, ModbusUnitID: unitID, IsActive: true,
			}, sourceIP)
			if err != nil {
				result.RowsSkipped++
				result.Errors = append(result.Errors, fmt.Sprintf("row %d: create device %s: %v", lineNo, externalID, err))
				continue
			}
			deviceByExternalID[plant.ID][externalID] = created.ID
			result.DevicesCreated++
		}
	}
	return result, nil
}

func firstNonEmptyCSV(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func parseFloatPtrCSV(value string) *float64 {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &v
}
