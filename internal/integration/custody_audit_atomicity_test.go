package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/sampling"
)

func TestCustodyHandoffAuditFailureRollsBackAggregate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	graph := f.createSourceGraph(t)
	now := time.Now().UTC()
	plan, err := f.sampling.CreatePlan(ctx, f.supervisor, sampling.CreatePlanCommand{
		FacilityID: graph.source.ID, StationID: graph.station.ID, AssignedUserID: f.field.UserID,
		WindowStart: now.Add(-time.Hour), WindowEnd: now.Add(time.Hour), RequiredBottles: 2,
		RequestID: "custody-rollback-plan",
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if err := f.sampling.PublishPlan(ctx, f.supervisor, plan.ID, "custody-rollback-publish"); err != nil {
		t.Fatalf("publish plan: %v", err)
	}
	sample, err := f.sampling.Collect(ctx, f.field, sampling.CollectCommand{
		PlanID: plan.ID, BottleCount: 2, CollectedAt: now, RequestID: "custody-rollback-collect",
	})
	if err != nil {
		t.Fatalf("collect sample: %v", err)
	}

	if _, err := f.store.DB().ExecContext(ctx, `
		CREATE TRIGGER reject_handoff_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.action = 'sample.handoff'
		BEGIN SELECT RAISE(ABORT, 'audit storage unavailable'); END`); err != nil {
		t.Fatalf("install audit failure: %v", err)
	}
	_, err = f.sampling.Handoff(ctx, f.field, sampling.HandoffCommand{
		SampleID: sample.ID, ToUserID: f.supervisor.UserID,
		OccurredAt: now.Add(time.Minute), RequestID: "custody-rollback-handoff",
	})
	if err == nil {
		t.Fatal("handoff unexpectedly succeeded while audit persistence was unavailable")
	}

	stored, err := f.store.Sample(ctx, f.store.DB(), f.field.OrganizationID, sample.ID)
	if err != nil {
		t.Fatalf("load sample after failed handoff: %v", err)
	}
	if stored.Status != domain.SampleCollected || stored.CustodianUserID != f.field.UserID || stored.Version != sample.Version {
		t.Fatalf("failed handoff changed sample aggregate: status=%s custodian=%s version=%d", stored.Status, stored.CustodianUserID, stored.Version)
	}
	var custodyEvents int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM custody_events WHERE sample_id = ?`, sample.ID).Scan(&custodyEvents); err != nil {
		t.Fatalf("count custody history: %v", err)
	}
	if custodyEvents != 1 {
		t.Fatalf("failed handoff left %d custody events, want only the collection event", custodyEvents)
	}
	var handoffAudits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id = 'custody-rollback-handoff'`).Scan(&handoffAudits); err != nil {
		t.Fatalf("count handoff audits: %v", err)
	}
	if handoffAudits != 0 {
		t.Fatalf("failed handoff left %d audit events", handoffAudits)
	}

	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_handoff_audit`); err != nil {
		t.Fatalf("restore audit storage: %v", err)
	}
	retried, err := f.sampling.Handoff(ctx, f.field, sampling.HandoffCommand{
		SampleID: sample.ID, ToUserID: f.supervisor.UserID,
		OccurredAt: now.Add(2 * time.Minute), RequestID: "custody-rollback-retry",
	})
	if err != nil {
		t.Fatalf("retry handoff after audit recovery: %v", err)
	}
	if retried.Status != domain.SampleInTransit || retried.CustodianUserID != f.supervisor.UserID {
		t.Fatalf("retry returned unexpected sample: %#v", retried)
	}
	if errors.Is(err, domain.ErrConflict) {
		t.Fatalf("retry was blocked by state left from the failed attempt: %v", err)
	}
}
