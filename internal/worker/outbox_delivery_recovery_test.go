package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	repository "github.com/vance1852/foodsafe-traceability/internal/repository/sqlite"
)

type controlledNotifier struct {
	result error
	calls  chan string
}

func (n *controlledNotifier) Deliver(_ context.Context, _ string, key string, _ []byte) error {
	n.calls <- key
	return n.result
}

func TestFailedOutboxDeliveryRemainsRecoverable(t *testing.T) {
	ctx := context.Background()
	store, err := repository.Open(ctx, filepath.Join(t.TempDir(), "outbox-worker.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Now().UTC()
	if err := store.CreateOrganization(ctx, "org-worker", "Worker Food Authority", now); err != nil {
		t.Fatalf("create organization: %v", err)
	}
	failedEvent := domain.OutboxEvent{
		ID: "failed-delivery", OrganizationID: "org-worker", Topic: "permit.suspended",
		AggregateType: "permit", AggregateID: "permit-7", IdempotencyKey: "permit-7:suspended",
		Payload: []byte(`{"permit_id":"permit-7"}`), Status: domain.OutboxPending,
		MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.InsertOutboxEvent(ctx, store.DB(), failedEvent); err != nil {
		t.Fatalf("insert failed-delivery event: %v", err)
	}

	failureNotifier := &controlledNotifier{result: errors.New("notification gateway unavailable"), calls: make(chan string, 1)}
	failureCtx, stopFailureWorker := context.WithCancel(ctx)
	failureDone := make(chan error, 1)
	go func() {
		failureDone <- New(store, nil, failureNotifier, slog.New(slog.NewTextHandler(io.Discard, nil)), "failure-worker", 10*time.Millisecond, time.Second, 1).Run(failureCtx)
	}()
	select {
	case key := <-failureNotifier.calls:
		if key != failedEvent.IdempotencyKey {
			t.Errorf("failed delivery key = %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not attempt failed notification")
	}

	var failedStatus string
	var failedAttempts int
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := store.DB().QueryRowContext(ctx, `SELECT status, attempt_count FROM outbox_events WHERE id = ?`, failedEvent.ID).Scan(&failedStatus, &failedAttempts); err != nil {
			t.Fatalf("read failed event: %v", err)
		}
		if failedStatus != string(domain.OutboxSending) || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	stopFailureWorker()
	<-failureDone
	if failedStatus != string(domain.OutboxRetry) || failedAttempts != 1 {
		t.Errorf("failed delivery persisted status=%s attempts=%d, want retry/1", failedStatus, failedAttempts)
	}

	successEvent := domain.OutboxEvent{
		ID: "successful-delivery", OrganizationID: "org-worker", Topic: "incident.reported",
		AggregateType: "incident", AggregateID: "incident-8", IdempotencyKey: "incident-8:reported",
		Payload: []byte(`{"incident_id":"incident-8"}`), Status: domain.OutboxPending,
		MaxAttempts: 3, AvailableAt: now, CreatedAt: now.Add(time.Millisecond), UpdatedAt: now,
	}
	if err := repository.InsertOutboxEvent(ctx, store.DB(), successEvent); err != nil {
		t.Fatalf("insert successful event: %v", err)
	}
	successNotifier := &controlledNotifier{calls: make(chan string, 1)}
	successCtx, stopSuccessWorker := context.WithCancel(ctx)
	successDone := make(chan error, 1)
	go func() {
		successDone <- New(store, nil, successNotifier, slog.New(slog.NewTextHandler(io.Discard, nil)), "success-worker", 10*time.Millisecond, time.Second, 1).Run(successCtx)
	}()
	select {
	case key := <-successNotifier.calls:
		if key != successEvent.IdempotencyKey {
			t.Errorf("successful delivery key = %q", key)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not attempt successful notification")
	}
	deadline = time.Now().Add(2 * time.Second)
	var successStatus string
	for {
		if err := store.DB().QueryRowContext(ctx, `SELECT status FROM outbox_events WHERE id = ?`, successEvent.ID).Scan(&successStatus); err != nil {
			t.Fatalf("read successful event: %v", err)
		}
		if successStatus == string(domain.OutboxDelivered) || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	stopSuccessWorker()
	<-successDone
	if successStatus != string(domain.OutboxDelivered) {
		t.Fatalf("successful delivery status = %s, want delivered", successStatus)
	}
}
