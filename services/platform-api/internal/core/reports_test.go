package core

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"
	"time"
)

func TestNormalizeReportRequest(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	got, err := normalizeReportRequest(ReportRequest{ReportType: " faults_maintenance ", From: from, To: to})
	if err != nil {
		t.Fatalf("normalizeReportRequest() error = %v", err)
	}
	if got.ReportType != ReportTypeFaultsMaintenance || !got.From.Equal(from) || !got.To.Equal(to) {
		t.Fatalf("normalized request = %+v", got)
	}
}

func TestNormalizeReportRequestRejectsInvalidRange(t *testing.T) {
	_, err := normalizeReportRequest(ReportRequest{ReportType: ReportTypeExecutive, From: time.Now(), To: time.Now()})
	if err != ErrReportInvalid {
		t.Fatalf("error = %v, want ErrReportInvalid", err)
	}
}

func TestBuildXLSXProducesReadableWorkbook(t *testing.T) {
	data, err := buildXLSX(context.Background(), [][]string{{"Plant", "Status"}, {"SITE-01", "OPERATIONAL"}})
	if err != nil {
		t.Fatalf("buildXLSX() error = %v", err)
	}
	if !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatal("workbook is not a zip archive")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	foundSheet := false
	for _, file := range archive.File {
		if file.Name == "xl/worksheets/sheet1.xml" {
			foundSheet = true
		}
	}
	if !foundSheet {
		t.Fatal("workbook has no worksheet")
	}
}
