package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/laboratory"
)

func TestExceedanceReviewFailurePreservesRetryableWorkflow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	graph := f.createSourceGraph(t)
	sample := f.createReceivedSample(t, graph)
	now := time.Now().UTC()
	result, err := f.lab.RecordResult(ctx, f.analyst, laboratory.RecordResultCommand{
		SampleID:        sample.ID,
		Parameter:       "lead",
		Value:           0.08,
		Unit:            "mg/kg",
		MethodCode:      "GB-5009.12",
		DetectionLimit:  0.001,
		RegulatoryLimit: 0.02,
		MeasuredAt:      now,
		RequestID:       "lab-atomic-record",
	})
	if err != nil {
		t.Fatalf("record exceedance result: %v", err)
	}
	if err := f.lab.Submit(ctx, f.analyst, result.ID, "lab-atomic-submit"); err != nil {
		t.Fatalf("submit exceedance result: %v", err)
	}

	if _, err := f.store.DB().ExecContext(ctx, `
		CREATE TRIGGER reject_result_incident
		BEFORE INSERT ON incidents
		WHEN NEW.originating_result_id IS NOT NULL
		BEGIN
			SELECT RAISE(FAIL, 'forced exceedance incident failure');
		END`); err != nil {
		t.Fatalf("install incident failure trigger: %v", err)
	}
	if _, err := f.lab.Review(ctx, f.supervisor, result.ID, true, "lab-atomic-failed-review"); err == nil {
		t.Fatal("review succeeded while exceedance incident creation was rejected")
	}

	storedResult, err := f.store.LabResult(ctx, f.store.DB(), f.supervisor.OrganizationID, result.ID)
	if err != nil {
		t.Fatalf("read result after failed review: %v", err)
	}
	storedSample, err := f.store.Sample(ctx, f.store.DB(), f.supervisor.OrganizationID, sample.ID)
	if err != nil {
		t.Fatalf("read sample after failed review: %v", err)
	}
	var failedIncidents, failedOutbox, failedAudits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents WHERE originating_result_id = ?`, result.ID).Scan(&failedIncidents); err != nil {
		t.Fatalf("count incidents after failed review: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE idempotency_key = ?`, "lab-exceedance:"+result.ID).Scan(&failedOutbox); err != nil {
		t.Fatalf("count outbox after failed review: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id = 'lab-atomic-failed-review'`).Scan(&failedAudits); err != nil {
		t.Fatalf("count audits after failed review: %v", err)
	}
	if storedResult.Status != domain.LabResultSubmitted || storedResult.ReviewerUserID != "" || storedSample.Status != domain.SampleReceived || failedIncidents != 0 || failedOutbox != 0 || failedAudits != 0 {
		t.Errorf("failed review left result=%s reviewer=%q sample=%s incidents=%d outbox=%d audits=%d", storedResult.Status, storedResult.ReviewerUserID, storedSample.Status, failedIncidents, failedOutbox, failedAudits)
	}

	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_result_incident`); err != nil {
		t.Fatalf("remove incident failure trigger: %v", err)
	}
	incidentID, err := f.lab.Review(ctx, f.supervisor, result.ID, true, "lab-atomic-retry-review")
	if err != nil {
		t.Fatalf("retry review after incident recovery: %v", err)
	}
	if incidentID == "" {
		t.Fatal("successful retry did not return an incident")
	}

	retriedResult, err := f.store.LabResult(ctx, f.store.DB(), f.supervisor.OrganizationID, result.ID)
	if err != nil {
		t.Fatalf("read result after retry: %v", err)
	}
	retriedSample, err := f.store.Sample(ctx, f.store.DB(), f.supervisor.OrganizationID, sample.ID)
	if err != nil {
		t.Fatalf("read sample after retry: %v", err)
	}
	var incidents, outbox, audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM incidents WHERE id = ? AND originating_result_id = ?`, incidentID, result.ID).Scan(&incidents); err != nil {
		t.Fatalf("count retry incident: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = ? AND idempotency_key = ?`, incidentID, "lab-exceedance:"+result.ID).Scan(&outbox); err != nil {
		t.Fatalf("count retry outbox: %v", err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id = 'lab-atomic-retry-review'`).Scan(&audits); err != nil {
		t.Fatalf("count retry audit: %v", err)
	}
	if retriedResult.Status != domain.LabResultApproved || retriedResult.ReviewerUserID != f.supervisor.UserID || retriedSample.Status != domain.SampleTested || incidents != 1 || outbox != 1 || audits != 1 {
		t.Fatalf("retry left result=%s reviewer=%q sample=%s incidents=%d outbox=%d audits=%d", retriedResult.Status, retriedResult.ReviewerUserID, retriedSample.Status, incidents, outbox, audits)
	}
}
