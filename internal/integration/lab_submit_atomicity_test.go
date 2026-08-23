package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/laboratory"
	"testing"
	"time"
)

func TestLabSubmitAuditFailureRollsBackTransition(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	sample := f.createReceivedSample(t, g)
	ctx := context.Background()
	now := time.Now().UTC()
	result, err := f.lab.RecordResult(ctx, f.analyst, laboratory.RecordResultCommand{SampleID: sample.ID, Parameter: "ph", Value: 7, Unit: "pH", MethodCode: "M-2", DetectionLimit: .01, RegulatoryLimit: 8, MeasuredAt: now, RequestID: "submit-record"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_submit_audit BEFORE INSERT ON audit_events WHEN NEW.action='lab_result.submit' BEGIN SELECT RAISE(FAIL,'submit audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.lab.Submit(ctx, f.analyst, result.ID, "submit-failed"); err == nil {
		t.Fatal("submit succeeded while audit was rejected")
	}
	var status string
	var audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT status FROM lab_results WHERE id=?`, result.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='submit-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.LabResultDraft) || audits != 0 {
		t.Fatalf("failed submit status=%s audits=%d", status, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_submit_audit`); err != nil {
		t.Fatal(err)
	}
	if err := f.lab.Submit(ctx, f.analyst, result.ID, "submit-retry"); err != nil {
		t.Fatalf("retry submit: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT status FROM lab_results WHERE id=?`, result.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(domain.LabResultSubmitted) {
		t.Fatalf("retry status=%s", status)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='submit-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
