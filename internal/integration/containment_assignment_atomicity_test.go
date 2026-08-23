package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/incident"
	"testing"
)

func TestContainmentAssignmentAuditFailureRollsBackAssignment(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	ctx := context.Background()
	reported, err := f.incidents.Report(ctx, f.supervisor, incident.ReportCommand{FacilityID: g.source.ID, Title: "Runoff", Description: "Contain runoff", Severity: domain.SeveritySignificant, RequestID: "contain-report"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := f.incidents.Claim(ctx, f.supervisor, reported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_contain_audit BEFORE INSERT ON audit_events WHEN NEW.action='containment.assign' BEGIN SELECT RAISE(FAIL,'contain audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = f.incidents.AssignContainment(ctx, f.supervisor, incident.AssignCommand{IncidentID: reported.ID, LeaseToken: claimed.LeaseToken, ResourceCode: "BARRIER-X", AssigneeUserID: f.field.UserID, RequestID: "contain-failed"})
	if err == nil {
		t.Fatal("assignment succeeded while audit was rejected")
	}
	var assignments, audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM containment_assignments WHERE incident_id=?`, reported.ID).Scan(&assignments); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='contain-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if assignments != 0 || audits != 0 {
		t.Fatalf("failed assignment left assignments=%d audits=%d", assignments, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_contain_audit`); err != nil {
		t.Fatal(err)
	}
	assignment, err := f.incidents.AssignContainment(ctx, f.supervisor, incident.AssignCommand{IncidentID: reported.ID, LeaseToken: claimed.LeaseToken, ResourceCode: "BARRIER-X", AssigneeUserID: f.field.UserID, RequestID: "contain-retry"})
	if err != nil {
		t.Fatalf("retry assignment: %v", err)
	}
	if assignment.Status != domain.AssignmentPending {
		t.Fatalf("retry status=%s", assignment.Status)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='contain-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
