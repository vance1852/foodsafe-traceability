package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/sampling"
	"testing"
	"time"
)

func TestSamplingPlanCreateAuditFailureRollsBackPlan(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_plan_create_audit BEFORE INSERT ON audit_events WHEN NEW.action='sampling_plan.create' BEGIN SELECT RAISE(FAIL,'plan create audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.sampling.CreatePlan(ctx, f.supervisor, sampling.CreatePlanCommand{FacilityID: g.source.ID, StationID: g.station.ID, AssignedUserID: f.field.UserID, WindowStart: now.Add(-time.Hour), WindowEnd: now.Add(time.Hour), RequiredBottles: 2, RequestID: "plan-failed"})
	if err == nil {
		t.Fatal("plan create succeeded while audit was rejected")
	}
	var plans, audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sampling_plans WHERE station_id=?`, g.station.ID).Scan(&plans); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='plan-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if plans != 0 || audits != 0 {
		t.Fatalf("failed create left plans=%d audits=%d", plans, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_plan_create_audit`); err != nil {
		t.Fatal(err)
	}
	plan, err := f.sampling.CreatePlan(ctx, f.supervisor, sampling.CreatePlanCommand{FacilityID: g.source.ID, StationID: g.station.ID, AssignedUserID: f.field.UserID, WindowStart: now.Add(-time.Hour), WindowEnd: now.Add(time.Hour), RequiredBottles: 2, RequestID: "plan-retry"})
	if err != nil {
		t.Fatalf("retry plan: %v", err)
	}
	if plan.ID == "" {
		t.Fatal("retry plan has empty id")
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='plan-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
