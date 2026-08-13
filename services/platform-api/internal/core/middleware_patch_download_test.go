package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPatchDownloadTokenOnlyAuthorizesItsPatchBeforeExpiry(t *testing.T) {
	s := &Service{patchDownloadKey: [32]byte{1, 2, 3}}
	now := time.Unix(1_786_600_000, 0)
	token := s.newPatchDownloadToken("patch-a", now)
	if !s.validPatchDownloadToken("patch-a", token, now.Add(4*time.Minute)) {
		t.Fatal("token should authorize its patch before expiry")
	}
	if s.validPatchDownloadToken("patch-b", token, now.Add(4*time.Minute)) {
		t.Fatal("token authorized a different patch")
	}
	if s.validPatchDownloadToken("patch-a", token, now.Add(6*time.Minute)) {
		t.Fatal("expired token was accepted")
	}
}

func TestPatchDownloadURLUsesCacheableZipPath(t *testing.T) {
	s := &Service{publicBaseURL: "https://ygate.example.com"}
	got := s.patchDownloadURL("patch-a", "token-a")
	want := "https://ygate.example.com/api/v1/admin/middleware-patches/patch-a/download/token-a/patch.zip"
	if got != want {
		t.Fatalf("patchDownloadURL() = %q, want %q", got, want)
	}
}

func TestPrewarmPatchDownloadReadsWholeFile(t *testing.T) {
	completed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 512<<10)))
		completed <- struct{}{}
	}))
	defer server.Close()

	if err := prewarmPatchDownload(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	select {
	case <-completed:
	default:
		t.Fatal("prewarm returned before the response completed")
	}
}

func TestRetryLegacyStageTimeoutOnce(t *testing.T) {
	attempts := 0
	err := retryLegacyStageTimeout(nil, func() error {
		attempts++
		if attempts == 1 {
			return errors.New("download patch: context deadline exceeded (Client.Timeout or context cancellation while reading body)")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("retryLegacyStageTimeout() err=%v attempts=%d, want success after 2", err, attempts)
	}
}

func TestRetryLegacyStageTimeoutExplainsTwoFailedAttempts(t *testing.T) {
	attempts := 0
	err := retryLegacyStageTimeout(nil, func() error {
		attempts++
		return errors.New("download patch: context deadline exceeded")
	})
	if err == nil || attempts != 2 || !strings.Contains(err.Error(), "60-second") {
		t.Fatalf("retryLegacyStageTimeout() err=%v attempts=%d, want legacy timeout detail after 2", err, attempts)
	}
}

func TestRetryLegacyStageTimeoutIncludesPrewarmFailure(t *testing.T) {
	err := retryLegacyStageTimeout(errors.New("HTTP 404 Not Found"), func() error {
		return errors.New("download patch: context deadline exceeded")
	})
	if err == nil || !strings.Contains(err.Error(), "edge prewarm failed: HTTP 404 Not Found") {
		t.Fatalf("retryLegacyStageTimeout() err=%v, want prewarm failure detail", err)
	}
}
