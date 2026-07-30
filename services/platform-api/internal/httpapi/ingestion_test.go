package httpapi

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIngestionBodyReaderSupportsGzip(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte(`{"data":[]}`))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "gzip")
	reader, closer, err := ingestionBodyReader(request)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil || string(decoded) != `{"data":[]}` {
		t.Fatalf("decoded=%q err=%v", decoded, err)
	}
}

func TestIngestionBodyReaderRejectsUnsupportedEncoding(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("Content-Encoding", "br")
	if _, _, err := ingestionBodyReader(request); err == nil {
		t.Fatal("expected unsupported encoding error")
	}
}

func TestIngestionCorrelationIDPreservesValidOrReplacesInvalidValue(t *testing.T) {
	for _, test := range []struct {
		input, want string
	}{
		{input: "gateway-01:batch_42", want: "gateway-01:batch_42"},
		{input: "invalid value with spaces"},
		{input: ""},
	} {
		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.Header.Set("X-Correlation-ID", test.input)
		response := httptest.NewRecorder()
		setIngestionCorrelationID(response, request)
		got := response.Header().Get("X-Correlation-ID")
		if test.want != "" && got != test.want {
			t.Fatalf("input=%q correlation=%q want=%q", test.input, got, test.want)
		}
		if test.want == "" && (!validCorrelationID(got) || got == test.input) {
			t.Fatalf("input=%q generated correlation=%q", test.input, got)
		}
	}
}
