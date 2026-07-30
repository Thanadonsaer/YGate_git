package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ygate/platform-api/internal/core"
)

func TestPlantRequestContracts(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		target  func() any
		decoded bool
	}{
		{
			name:   "create accepts documented fields",
			body:   `{"organizationId":"10000000-0000-4000-8000-000000000001","code":"A","name":"Plant","timezone":"UTC"}`,
			target: func() any { return &createPlantRequest{} }, decoded: true,
		},
		{
			name:   "create rejects update-only isActive",
			body:   `{"code":"A","name":"Plant","timezone":"UTC","isActive":true}`,
			target: func() any { return &createPlantRequest{} }, decoded: false,
		},
		{
			name:   "update accepts documented fields",
			body:   `{"code":"A","name":"Plant","timezone":"UTC","isActive":false}`,
			target: func() any { return &updatePlantRequest{} }, decoded: true,
		},
		{
			name:   "update rejects immutable organizationId",
			body:   `{"organizationId":null,"code":"A","name":"Plant","timezone":"UTC","isActive":true}`,
			target: func() any { return &updatePlantRequest{} }, decoded: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if decoded := decodeJSON(recorder, request, test.target(), 32<<10); decoded != test.decoded {
				t.Fatalf("decoded=%v status=%d body=%q", decoded, recorder.Code, recorder.Body.String())
			}
			if !test.decoded && recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d", recorder.Code)
			}
		})
	}
}

func TestWritePlantError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{"invalid", core.ErrInvalid, http.StatusBadRequest},
		{"forbidden", core.ErrForbidden, http.StatusForbidden},
		{"not found", core.ErrNotFound, http.StatusNotFound},
		{"conflict", core.ErrConflict, http.StatusConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			if !writePlantError(recorder, test.err) {
				t.Fatal("error was not handled")
			}
			if recorder.Code != test.status {
				t.Fatalf("status=%d", recorder.Code)
			}
		})
	}
}
