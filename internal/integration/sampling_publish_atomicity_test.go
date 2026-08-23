package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/sampling"
	"testing"
	"time"
)

func TestSamplingPublicationAuditFailureRollsBackPlan(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	ctx := context.Background()
	now := time.Now().UTC()
	plan, err := f.sampling.CreatePlan(ctx, f.supervisor, sampling.CreatePlanCommand{FacilityID: g.source.ID, StationID: g.station.ID, AssignedUserID: f.field.UserID, WindowStart: now.Add(-time.Hour), WindowEnd: now.Add(time.Hour), RequiredBottles: 2, RequestID: "publish-create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_publish_audit BEFORE INSERT ON audit_events WHEN NEW.action='sampling_plan.publish' BEGIN SELECT RAISE(FAIL,'publish audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.sampling.PublishPlan(ctx, f.supervisor, plan.ID, "publish-failed"); err == nil {
		t.Fatal("publish succeeded while audit was rejected")
	}
	var status string
	var audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT status FROM sampling_plans WHERE id=?`, plan.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='publish-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.PlanDraft) || audits != 0 {
		t.Fatalf("failed publish status=%s audits=%d", status, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_publish_audit`); err != nil {
		t.Fatal(err)
	}
	if err := f.sampling.PublishPlan(ctx, f.supervisor, plan.ID, "publish-retry"); err != nil {
		t.Fatalf("retry publish: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT status FROM sampling_plans WHERE id=?`, plan.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.PlanPublished) {
		t.Fatalf("retry status=%s", status)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='publish-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
