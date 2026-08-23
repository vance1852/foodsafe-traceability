package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/permit"
)

func TestPermitSuspensionAuditFailureRollsBackPublication(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	graph := f.createSourceGraph(t)
	now := time.Now().UTC()
	created, err := f.permits.Create(ctx, f.supervisor, permit.CreateCommand{
		FacilityID:             graph.source.ID,
		HolderName:             "Regional Cold Storage",
		Reference:              "SUSPEND-ATOMIC-1",
		ValidFrom:              now.Add(-time.Hour),
		ValidUntil:             now.Add(24 * time.Hour),
		DailyVolumeLimitLiters: 500,
		RequestID:              "permit-suspend-create",
	})
	if err != nil {
		t.Fatalf("create permit: %v", err)
	}
	if err := f.permits.Activate(ctx, f.supervisor, created.ID, "permit-suspend-activate"); err != nil {
		t.Fatalf("activate permit: %v", err)
	}

	if _, err := f.store.DB().ExecContext(ctx, `
		CREATE TRIGGER reject_permit_suspend_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.action = 'permit.suspend'
		BEGIN
			SELECT RAISE(FAIL, 'forced permit suspension audit failure');
		END`); err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}
	if err := f.permits.Suspend(ctx, f.supervisor, created.ID, "temperature control breach", "permit-suspend-failed"); err == nil {
		t.Fatal("suspend permit succeeded while its audit write was rejected")
	}

	failedPermit, err := f.store.Permit(ctx, f.store.DB(), f.supervisor.OrganizationID, created.ID)
	if err != nil {
		t.Fatalf("read permit after failed suspension: %v", err)
	}
	var failedOutbox, failedAudits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = ? AND topic = 'permit.suspended'`, created.ID).Scan(&failedOutbox); err != nil {
		t.Fatalf("count outbox after failed suspension: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id = 'permit-suspend-failed'`).Scan(&failedAudits); err != nil {
		t.Fatalf("count audit after failed suspension: %v", err)
	}
	if failedPermit.Status != domain.PermitActive || failedOutbox != 0 || failedAudits != 0 {
		t.Errorf("failed suspension left permit=%s outbox=%d audits=%d", failedPermit.Status, failedOutbox, failedAudits)
	}

	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_permit_suspend_audit`); err != nil {
		t.Fatalf("remove audit failure trigger: %v", err)
	}
	if err := f.permits.Suspend(ctx, f.supervisor, created.ID, "temperature control breach", "permit-suspend-retry"); err != nil {
		t.Fatalf("retry suspension after audit recovery: %v", err)
	}

	retriedPermit, err := f.store.Permit(ctx, f.store.DB(), f.supervisor.OrganizationID, created.ID)
	if err != nil {
		t.Fatalf("read permit after retry: %v", err)
	}
	var deliveredOutbox, deliveredAudits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = ? AND topic = 'permit.suspended'`, created.ID).Scan(&deliveredOutbox); err != nil {
		t.Fatalf("count outbox after retry: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id = 'permit-suspend-retry'`).Scan(&deliveredAudits); err != nil {
		t.Fatalf("count audit after retry: %v", err)
	}
	if retriedPermit.Status != domain.PermitSuspended || deliveredOutbox != 1 || deliveredAudits != 1 {
		t.Fatalf("successful retry left permit=%s outbox=%d audits=%d", retriedPermit.Status, deliveredOutbox, deliveredAudits)
	}
}
