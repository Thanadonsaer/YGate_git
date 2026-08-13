package core

import (
	"testing"
	"time"
)

func TestMiddlewareUpdateJobStoreTracksEachGatewayAndElapsedTime(t *testing.T) {
	store := newMiddlewareUpdateJobStore()
	job := store.create("stage", "patch-1", []string{"gw-1", "gw-2"})

	store.startItem(job.ID, "gw-1")
	time.Sleep(2 * time.Millisecond)
	store.finishItem(job.ID, "gw-1", true, "")
	store.finishItem(job.ID, "gw-2", false, "download patch failed")

	got, ok := store.get(job.ID)
	if !ok {
		t.Fatal("job not found")
	}
	if got.Status != "failed" {
		t.Fatalf("job status = %q, want failed", got.Status)
	}
	if got.Items["gw-1"].Status != "succeeded" || got.Items["gw-1"].DurationMs < 1 {
		t.Fatalf("successful item = %#v, want succeeded with elapsed time", got.Items["gw-1"])
	}
	if got.Items["gw-2"].Status != "failed" || got.Items["gw-2"].Error != "download patch failed" {
		t.Fatalf("failed item = %#v", got.Items["gw-2"])
	}
}
