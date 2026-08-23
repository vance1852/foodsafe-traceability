package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/incident"
	"github.com/vance1852/foodsafe-traceability/internal/remediation"
)

func TestInvalidRemediationActionRollsBackWholePlan(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	graph := f.createSourceGraph(t)
	reported, err := f.incidents.Report(ctx, f.field, incident.ReportCommand{
		FacilityID: graph.source.ID, Title: "Cold storage sanitation recovery",
		Description: "A failed sanitation cycle requires coordinated corrective work",
		Severity:    domain.SeveritySignificant, RequestID: "remediation-batch-incident",
	})
	if err != nil {
		t.Fatalf("report incident: %v", err)
	}
	command := remediation.CreatePlanCommand{
		IncidentID: reported.ID, Title: "Restore sanitation controls",
		Objective:   "Complete cleaning and independent verification before release",
		BudgetCents: 250000, RequestID: "remediation-batch-invalid",
		Actions: []remediation.CreateAction{
			{IdempotencyKey: "deep-clean", Description: "Deep clean the affected production line"},
			{IdempotencyKey: "verify-swabs", Description: "bad"},
		},
	}
	if _, err := f.remediation.CreatePlan(ctx, f.supervisor, command); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid action error = %v, want validation", err)
	}
	assertRemediationAggregateCounts(t, f, reported.ID, 0, 0, 0)

	command.RequestID = "remediation-batch-retry"
	command.Actions[1].Description = "Collect verification swabs after cleaning"
	plan, err := f.remediation.CreatePlan(ctx, f.supervisor, command)
	if err != nil {
		t.Fatalf("retry corrected plan: %v", err)
	}
	if plan.IncidentID != reported.ID || plan.Status != domain.RemediationDraft {
		t.Fatalf("corrected plan = %#v", plan)
	}
	assertRemediationAggregateCounts(t, f, reported.ID, 1, 2, 1)
}

func assertRemediationAggregateCounts(t *testing.T, f *fixture, incidentID string, wantPlans, wantActions, wantAudits int) {
	t.Helper()
	var plans, actions, audits int
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM remediation_plans WHERE incident_id = ?`, incidentID).Scan(&plans); err != nil {
		t.Fatalf("count remediation plans: %v", err)
	}
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM remediation_actions WHERE plan_id IN (SELECT id FROM remediation_plans WHERE incident_id = ?)`, incidentID).Scan(&actions); err != nil {
		t.Fatalf("count remediation actions: %v", err)
	}
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action = 'remediation_plan.create' AND object_id IN (SELECT id FROM remediation_plans WHERE incident_id = ?)`, incidentID).Scan(&audits); err != nil {
		t.Fatalf("count remediation audits: %v", err)
	}
	if plans != wantPlans || actions != wantActions || audits != wantAudits {
		t.Fatalf("remediation aggregate counts = plans:%d actions:%d audits:%d, want %d/%d/%d", plans, actions, audits, wantPlans, wantActions, wantAudits)
	}
}
