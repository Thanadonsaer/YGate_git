package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"ygate/platform-api/internal/ingestion"
)

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
