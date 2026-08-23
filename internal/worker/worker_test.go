package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	repository "github.com/vance1852/foodsafe-traceability/internal/repository/sqlite"
	"log/slog"
)

type fakeNotifier struct {
	mu        sync.Mutex
	fails     int
	delivered []string
}

func (n *fakeNotifier) Deliver(ctx context.Context, topic, idempotencyKey string, payload []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.delivered = append(n.delivered, idempotencyKey)
	if n.fails > 0 {
		n.fails--
		return errors.New("notification gateway unavailable")
	}
	return nil
}

func newRuntime(t *testing.T, notifier Notifier) (*Runtime, *repository.Store) {
	t.Helper()
	path := t.TempDir() + "/worker-test.db"
	store, err := repository.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.CreateOrganization(context.Background(), "org-1", "River Authority", time.Now().UTC()); err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}
	return New(store, nil, notifier, slog.Default(), "owner", time.Millisecond, time.Minute, 1), store
}

func seedOutboxEvent(t *testing.T, store *repository.Store, now time.Time) domain.OutboxEvent {
	t.Helper()
	event := domain.OutboxEvent{
		ID: "outbox-1", OrganizationID: "org-1", Topic: "incident.reported",
		AggregateType: "incident", AggregateID: "i1", IdempotencyKey: "key-1",
		Payload: []byte(`{"incident":"i1"}`), Status: domain.OutboxPending,
		MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.InsertOutboxEvent(context.Background(), store.DB(), event); err != nil {
		t.Fatalf("InsertOutboxEvent() error = %v", err)
	}
	return event
}

func outboxState(t *testing.T, store *repository.Store) (string, int, string) {
	t.Helper()
	var status string
	var attempt int
	var lastError string
	if err := store.DB().QueryRow(`SELECT status, attempt_count, last_error FROM outbox_events WHERE id = 'outbox-1'`).Scan(&status, &attempt, &lastError); err != nil {
		t.Fatalf("scan outbox: %v", err)
	}
	return status, attempt, lastError
}

// TestProcessOneFailedDeliveryStaysRecoverable guards the regression where a failed
// notification was marked delivered before the notifier was even called.
func TestProcessOneFailedDeliveryStaysRecoverable(t *testing.T) {
	notifier := &fakeNotifier{fails: 1}
	runtime, store := newRuntime(t, notifier)
	seedOutboxEvent(t, store, time.Now().UTC().Add(-time.Hour))

	if err := runtime.processOne(context.Background(), "owner-1"); err == nil {
		t.Fatal("processOne() expected error from failed delivery, got nil")
	}

	status, attempt, lastError := outboxState(t, store)
	if status != string(domain.OutboxRetry) {
		t.Fatalf("after failed delivery status = %q, want %q", status, domain.OutboxRetry)
	}
	if attempt != 1 {
		t.Fatalf("after failed delivery attempt_count = %d, want 1", attempt)
	}
	if lastError == "" {
		t.Fatal("last_error should record the delivery failure")
	}
	if len(notifier.delivered) != 1 {
		t.Fatalf("notifier called %d times, want 1", len(notifier.delivered))
	}
}

// TestProcessOneDeliveredOnlyAfterNotifierSucceeds confirms an event reaches delivered
// only once the notifier confirms success, and that success does not inflate attempt_count.
func TestProcessOneDeliveredOnlyAfterNotifierSucceeds(t *testing.T) {
	notifier := &fakeNotifier{fails: 1}
	runtime, store := newRuntime(t, notifier)
	seedOutboxEvent(t, store, time.Now().UTC().Add(-time.Hour))

	// First delivery fails: event must remain recoverable.
	if err := runtime.processOne(context.Background(), "owner-1"); err == nil {
		t.Fatal("first processOne() expected error, got nil")
	}
	if status, attempt, _ := outboxState(t, store); status != string(domain.OutboxRetry) || attempt != 1 {
		t.Fatalf("after failure status=%s attempt=%d, want retry/1", status, attempt)
	}

	// Make the event immediately claimable again (bypass backoff) and succeed.
	if _, err := store.DB().Exec(`UPDATE outbox_events SET available_at = ? WHERE id = 'outbox-1'`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("reset available_at: %v", err)
	}
	if err := runtime.processOne(context.Background(), "owner-1"); err != nil {
		t.Fatalf("second processOne() error = %v", err)
	}

	status, attempt, _ := outboxState(t, store)
	if status != string(domain.OutboxDelivered) {
		t.Fatalf("after successful delivery status = %q, want %q", status, domain.OutboxDelivered)
	}
	if attempt != 1 {
		t.Fatalf("success must not change attempt_count, got %d, want 1", attempt)
	}
	if len(notifier.delivered) != 2 {
		t.Fatalf("notifier called %d times, want 2", len(notifier.delivered))
	}
}
