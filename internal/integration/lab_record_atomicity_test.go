package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/laboratory"
	"testing"
	"time"
)

func TestLabResultRecordAuditFailureRollsBackResult(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	sample := f.createReceivedSample(t, g)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_lab_record_audit BEFORE INSERT ON audit_events WHEN NEW.action='lab_result.record' BEGIN SELECT RAISE(FAIL,'lab record audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.lab.RecordResult(ctx, f.analyst, laboratory.RecordResultCommand{SampleID: sample.ID, Parameter: "nitrate", Value: 1.2, Unit: "mg/L", MethodCode: "M-1", DetectionLimit: .01, RegulatoryLimit: 2, MeasuredAt: now, RequestID: "lab-record-failed"})
	if err == nil {
		t.Fatal("lab result succeeded while audit was rejected")
	}
	var results, audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM lab_results WHERE sample_id=?`, sample.ID).Scan(&results); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='lab-record-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if results != 0 || audits != 0 {
		t.Fatalf("failed result left results=%d audits=%d", results, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_lab_record_audit`); err != nil {
		t.Fatal(err)
	}
	result, err := f.lab.RecordResult(ctx, f.analyst, laboratory.RecordResultCommand{SampleID: sample.ID, Parameter: "nitrate", Value: 1.2, Unit: "mg/L", MethodCode: "M-1", DetectionLimit: .01, RegulatoryLimit: 2, MeasuredAt: now, RequestID: "lab-record-retry"})
	if err != nil {
		t.Fatalf("retry result: %v", err)
	}
	if result.ID == "" {
		t.Fatal("retry result empty")
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='lab-record-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
