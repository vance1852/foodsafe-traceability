package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/incident"
	"testing"
)

func TestIncidentReportAuditFailureRollsBackPublication(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	ctx := context.Background()
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_incident_audit BEFORE INSERT ON audit_events WHEN NEW.action='incident.report' BEGIN SELECT RAISE(FAIL,'incident audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.incidents.Report(ctx, f.supervisor, incident.ReportCommand{FacilityID: g.source.ID, Title: "Chemical spill", Description: "Contain affected storage area", Severity: domain.SeveritySignificant, RequestID: "incident-failed"})
	if err == nil {
		t.Fatal("incident report succeeded while audit was rejected")
	}
	var incidents, outbox, audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents WHERE facility_id=?`, g.source.ID).Scan(&incidents); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE topic='incident.reported'`).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='incident-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if incidents != 0 || outbox != 0 || audits != 0 {
		t.Fatalf("failed report left incidents=%d outbox=%d audits=%d", incidents, outbox, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_incident_audit`); err != nil {
		t.Fatal(err)
	}
	reported, err := f.incidents.Report(ctx, f.supervisor, incident.ReportCommand{FacilityID: g.source.ID, Title: "Chemical spill", Description: "Contain affected storage area", Severity: domain.SeveritySignificant, RequestID: "incident-retry"})
	if err != nil {
		t.Fatalf("retry report: %v", err)
	}
	if reported.Status != domain.IncidentReported {
		t.Fatalf("retry status=%s", reported.Status)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='incident-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
