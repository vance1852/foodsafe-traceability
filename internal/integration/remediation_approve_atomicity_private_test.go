package integration_test

import (
	"context"
	"testing"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/incident"
	"github.com/vance1852/foodsafe-traceability/internal/remediation"
)

func TestRemediationApprovalRollsBackWhenAuditFails(t *testing.T) {
	f := newFixture(t)
	graph := f.createSourceGraph(t)
	reported, err := f.incidents.Report(context.Background(), f.field, incident.ReportCommand{
		FacilityID: graph.source.ID,
		Title:      "Allergen response",
		Description: "A supplier lot requires corrective action before release",
		Severity:   domain.SeveritySignificant,
		RequestID:  "private-incident-approve",
	})
	if err != nil {
		t.Fatalf("report incident: %v", err)
	}
	plan, err := f.remediation.CreatePlan(context.Background(), f.supervisor, remediation.CreatePlanCommand{
		IncidentID: reported.ID,
		Title:      "Supplier lot correction",
		Objective:  "Hold and verify the affected lot",
		BudgetCents: 150000,
		Actions: []remediation.CreateAction{{
			IdempotencyKey: "hold-lot",
			Description:    "Place affected supplier lot on hold",
		}},
		RequestID: "private-plan-create",
	})
	if err != nil {
		t.Fatalf("create remediation plan: %v", err)
	}

	if _, err := f.store.DB().ExecContext(context.Background(), `
		CREATE TRIGGER fail_remediation_approval_audit
		BEFORE INSERT ON audit_events
		WHEN NEW.action = 'remediation_plan.approve'
		BEGIN SELECT RAISE(ABORT, 'audit sink unavailable'); END`); err != nil {
		t.Fatalf("install audit failure: %v", err)
	}
	if err := f.remediation.Approve(context.Background(), f.supervisor, plan.ID, "private-approve-failing"); err == nil {
		t.Fatal("approval unexpectedly succeeded while audit was unavailable")
	}
	var status string
	if err := f.store.DB().QueryRowContext(context.Background(), `SELECT status FROM remediation_plans WHERE id = ?`, plan.ID).Scan(&status); err != nil {
		t.Fatalf("read failed approval status: %v", err)
	}
	if status != string(domain.RemediationDraft) {
		t.Fatalf("failed approval left status %q", status)
	}
	var failedAuditCount int
	if err := f.store.DB().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE request_id = ?`, "private-approve-failing").Scan(&failedAuditCount); err != nil {
		t.Fatalf("count failed approval audits: %v", err)
	}
	if failedAuditCount != 0 {
		t.Fatalf("failed approval wrote %d audit events", failedAuditCount)
	}

	if _, err := f.store.DB().ExecContext(context.Background(), `DROP TRIGGER fail_remediation_approval_audit`); err != nil {
		t.Fatalf("remove audit failure: %v", err)
	}
	if err := f.remediation.Approve(context.Background(), f.supervisor, plan.ID, "private-approve-retry"); err != nil {
		t.Fatalf("retry approval: %v", err)
	}
	if err := f.store.DB().QueryRowContext(context.Background(), `SELECT status FROM remediation_plans WHERE id = ?`, plan.ID).Scan(&status); err != nil {
		t.Fatalf("read successful approval status: %v", err)
	}
	if status != string(domain.RemediationApproved) {
		t.Fatalf("successful approval status = %q", status)
	}
	var successfulAuditCount int
	if err := f.store.DB().QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE request_id = ? AND action = 'remediation_plan.approve'`, "private-approve-retry").Scan(&successfulAuditCount); err != nil {
		t.Fatalf("count successful approval audits: %v", err)
	}
	if successfulAuditCount != 1 {
		t.Fatalf("successful approval wrote %d audit events", successfulAuditCount)
	}
}
