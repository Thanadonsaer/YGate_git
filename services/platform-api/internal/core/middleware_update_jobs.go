package core

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"ygate/platform-api/internal/auth"
)

type MiddlewareUpdateJobItem struct {
	MiddlewareID string    `json:"middlewareId"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	StartedAt    time.Time `json:"startedAt,omitempty"`
	FinishedAt   time.Time `json:"finishedAt,omitempty"`
	DurationMs   int64     `json:"durationMs,omitempty"`
}

type MiddlewareUpdateJob struct {
	ID         string                             `json:"id"`
	Action     string                             `json:"action"`
	PatchID    string                             `json:"patchId,omitempty"`
	Status     string                             `json:"status"`
	CreatedAt  time.Time                          `json:"createdAt"`
	StartedAt  time.Time                          `json:"startedAt,omitempty"`
	FinishedAt time.Time                          `json:"finishedAt,omitempty"`
	DurationMs int64                              `json:"durationMs,omitempty"`
	Items      map[string]MiddlewareUpdateJobItem `json:"items"`
}

type middlewareUpdateJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*MiddlewareUpdateJob
}

func newMiddlewareUpdateJobStore() *middlewareUpdateJobStore {
	return &middlewareUpdateJobStore{jobs: make(map[string]*MiddlewareUpdateJob)}
}

func (s *middlewareUpdateJobStore) create(action, patchID string, middlewareIDs []string) MiddlewareUpdateJob {
	id, _ := newUUID()
	now := time.Now().UTC()
	job := &MiddlewareUpdateJob{ID: uuidString(id), Action: action, PatchID: patchID, Status: "running", CreatedAt: now, StartedAt: now, Items: make(map[string]MiddlewareUpdateJobItem, len(middlewareIDs))}
	for _, middlewareID := range middlewareIDs {
		job.Items[middlewareID] = MiddlewareUpdateJobItem{MiddlewareID: middlewareID, Status: "pending"}
	}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	return cloneMiddlewareUpdateJob(job)
}

func (s *middlewareUpdateJobStore) startItem(jobID, middlewareID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.jobs[jobID]; job != nil {
		item := job.Items[middlewareID]
		item.Status, item.StartedAt = "running", time.Now().UTC()
		job.Items[middlewareID] = item
	}
}

func (s *middlewareUpdateJobStore) finishItem(jobID, middlewareID string, ok bool, itemErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return
	}
	item := job.Items[middlewareID]
	item.FinishedAt = time.Now().UTC()
	if !item.StartedAt.IsZero() {
		item.DurationMs = item.FinishedAt.Sub(item.StartedAt).Milliseconds()
	}
	if ok {
		item.Status = "succeeded"
	} else {
		item.Status, item.Error = "failed", itemErr
	}
	job.Items[middlewareID] = item
	job.Status = "succeeded"
	for _, candidate := range job.Items {
		if candidate.Status == "pending" || candidate.Status == "running" {
			job.Status = "running"
			return
		}
		if candidate.Status == "failed" {
			job.Status = "failed"
		}
	}
	job.FinishedAt = time.Now().UTC()
	job.DurationMs = job.FinishedAt.Sub(job.StartedAt).Milliseconds()
}

func (s *middlewareUpdateJobStore) get(jobID string) (MiddlewareUpdateJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return MiddlewareUpdateJob{}, false
	}
	return cloneMiddlewareUpdateJob(job), true
}

func cloneMiddlewareUpdateJob(job *MiddlewareUpdateJob) MiddlewareUpdateJob {
	copyJob := *job
	copyJob.Items = make(map[string]MiddlewareUpdateJobItem, len(job.Items))
	for id, item := range job.Items {
		copyJob.Items[id] = item
	}
	return copyJob
}

func (s *Service) CreateMiddlewareUpdateBatch(ctx context.Context, principal auth.Principal, action, patchID string, middlewareIDs []string, sourceIP *netip.Addr) (MiddlewareUpdateJob, error) {
	if err := s.requireGlobalPermission(ctx, principal, "update", "middleware_patch"); err != nil {
		return MiddlewareUpdateJob{}, err
	}
	if action != "stage" && action != "apply" {
		return MiddlewareUpdateJob{}, ErrInvalid
	}
	if action == "stage" && patchID == "" {
		return MiddlewareUpdateJob{}, ErrMiddlewarePatchNotFound
	}
	if len(middlewareIDs) == 0 || len(middlewareIDs) > 100 {
		return MiddlewareUpdateJob{}, ErrInvalid
	}
	job := s.updateJobs.create(action, patchID, middlewareIDs)
	go func() {
		var wg sync.WaitGroup
		for _, middlewareID := range middlewareIDs {
			middlewareID := middlewareID
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.updateJobs.startItem(job.ID, middlewareID)
				var err error
				if action == "stage" {
					err = s.StageMiddlewareUpdate(context.Background(), principal, middlewareID, patchID, sourceIP)
				} else {
					err = s.ApplyMiddlewareUpdate(context.Background(), principal, middlewareID, sourceIP)
				}
				s.updateJobs.finishItem(job.ID, middlewareID, err == nil, errorString(err))
			}()
		}
		wg.Wait()
	}()
	return job, nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Service) MiddlewareUpdateBatch(ctx context.Context, principal auth.Principal, jobID string) (MiddlewareUpdateJob, error) {
	if err := s.requireGlobalPermission(ctx, principal, "read", "middleware_patch"); err != nil {
		return MiddlewareUpdateJob{}, err
	}
	job, ok := s.updateJobs.get(jobID)
	if !ok {
		return MiddlewareUpdateJob{}, fmt.Errorf("%w: middleware update job not found", ErrNotFound)
	}
	return job, nil
}
