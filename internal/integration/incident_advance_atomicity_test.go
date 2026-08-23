package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/incident"
	"testing"
)

func TestIncidentAdvanceAuditFailureRollsBackTransition(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	ctx := context.Background()
	reported, err := f.incidents.Report(ctx, f.supervisor, incident.ReportCommand{FacilityID: g.source.ID, Title: "Spill incident", Description: "Contain contaminated storage", Severity: domain.SeveritySignificant, RequestID: "advance-report"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := f.incidents.Claim(ctx, f.supervisor, reported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_advance_audit BEFORE INSERT ON audit_events WHEN NEW.action='incident.advance' BEGIN SELECT RAISE(FAIL,'advance audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.incidents.Advance(ctx, f.supervisor, reported.ID, claimed.LeaseToken, domain.IncidentAssessing, "advance-failed"); err == nil {
		t.Fatal("advance succeeded while audit was rejected")
	}
	var status string
	var audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT status FROM incidents WHERE id=?`, reported.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='advance-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.IncidentReported) || audits != 0 {
		t.Fatalf("failed advance status=%s audits=%d", status, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_advance_audit`); err != nil {
		t.Fatal(err)
	}
	if err := f.incidents.Advance(ctx, f.supervisor, reported.ID, claimed.LeaseToken, domain.IncidentAssessing, "advance-retry"); err != nil {
		t.Fatalf("retry advance: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT status FROM incidents WHERE id=?`, reported.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.IncidentAssessing) {
		t.Fatalf("retry status=%s", status)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='advance-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
