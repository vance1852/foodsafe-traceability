package worker_test

import (
	"context"
	"errors"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	repository "github.com/vance1852/foodsafe-traceability/internal/repository/sqlite"
	"path/filepath"
	"testing"
	"time"
)

func TestCancelledWorkerClaimDoesNotPersistLease(t *testing.T) {
	ctx := context.Background()
	store, err := repository.Open(ctx, filepath.Join(t.TempDir(), "cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC()
	if err := store.CreateOrganization(ctx, "org-cancel", "Cancellation Authority", now); err != nil {
		t.Fatal(err)
	}
	event := domain.OutboxEvent{ID: "cancel-event", OrganizationID: "org-cancel", Topic: "incident.reported", AggregateType: "incident", AggregateID: "incident-cancel", IdempotencyKey: "cancel-key", Payload: []byte(`{}`), Status: domain.OutboxPending, MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
	if err := repository.InsertOutboxEvent(ctx, store.DB(), event); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.ClaimOutboxEvent(cancelled, "cancel-worker", "cancel-token", now, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled claim error=%v", err)
	}
	var status string
	if err := store.DB().QueryRowContext(ctx, `SELECT status FROM outbox_events WHERE id = ?`, event.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.OutboxPending) {
		t.Fatalf("cancelled claim persisted status=%s", status)
	}
}
