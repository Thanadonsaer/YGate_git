package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"ygate/platform-api/internal/ingestion"
)

const maxIngestionBody = 1 << 20

func rawIngestionHandler(service *ingestion.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setIngestionCorrelationID(w, r)
		client, err := service.Authenticate(r.Context(), r.Header.Get("X-Api-Key"))
		if errors.Is(err, ingestion.ErrUnauthenticated) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if err != nil {
			log.Printf("middleware authentication failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		reader, closeReader, err := ingestionBodyReader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
			return
		}
		if closeReader != nil {
			defer closeReader.Close()
		}
		raw, err := io.ReadAll(io.LimitReader(reader, maxIngestionBody+1))
		if err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if len(raw) > maxIngestionBody {
			http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		var batch ingestion.RawBatch
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&batch); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		result, err := service.IngestRaw(r.Context(), client, r.Header.Get("Idempotency-Key"), raw, batch, time.Now())
		switch {
		case errors.Is(err, ingestion.ErrInvalidBatch):
			http.Error(w, "invalid raw register batch", http.StatusBadRequest)
		case errors.Is(err, ingestion.ErrIdempotencyConflict):
			http.Error(w, "idempotency key conflict", http.StatusConflict)
		case err != nil:
			log.Printf("raw register ingestion failed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		default:
			writeJSON(w, http.StatusAccepted, result)
		}
	}
}

func setIngestionCorrelationID(w http.ResponseWriter, r *http.Request) {
	value := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
	if !validCorrelationID(value) {
		buffer := make([]byte, 16)
		if _, err := rand.Read(buffer); err == nil {
			value = hex.EncodeToString(buffer)
		} else {
			value = fmt.Sprintf("request-%d", time.Now().UnixNano())
		}
	}
	w.Header().Set("X-Correlation-ID", value)
}

func validCorrelationID(value string) bool {
	if len(value) == 0 || len(value) > 100 {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("._:-", character)) {
			return false
		}
	}
	return true
}

func ingestionBodyReader(r *http.Request) (io.Reader, io.Closer, error) {
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))) {
	case "", "identity":
		return r.Body, nil, nil
	case "gzip":
		reader, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, nil, errors.New("invalid gzip request")
		}
		return reader, reader, nil
	default:
		return nil, nil, errors.New("unsupported content encoding")
	}
}
