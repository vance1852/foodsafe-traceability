package integration_test

import (
    "context"
    "testing"
    "time"

    "github.com/vance1852/foodsafe-traceability/internal/domain"
    "github.com/vance1852/foodsafe-traceability/internal/sampling"
)

func TestSamplingHandoffRollsBackWhenAuditFails(t *testing.T) {
    f := newFixture(t)
    graph := f.createSourceGraph(t)
    sample := f.createReceivedSample(t, graph)
    if _, err := f.store.DB().Exec(`UPDATE samples SET status = 'collected', custodian_user_id = ?, received_at = NULL WHERE id = ?`, f.field.UserID, sample.ID); err != nil { t.Fatal(err) }
    if _, err := f.store.DB().Exec(`CREATE TRIGGER fail_sampling_handoff_audit BEFORE INSERT ON audit_events WHEN NEW.action = 'sample.handoff' BEGIN SELECT RAISE(ABORT, 'audit sink unavailable'); END`); err != nil { t.Fatal(err) }
    err := func() error { _, e := f.sampling.Handoff(context.Background(), f.field, sampling.HandoffCommand{SampleID: sample.ID, ToUserID: f.analyst.UserID, OccurredAt: time.Now().UTC(), RequestID: "private-handoff-failing"}); return e }()
    if err == nil { t.Fatal("handoff unexpectedly succeeded") }
    var status string
    if err := f.store.DB().QueryRow(`SELECT status FROM samples WHERE id = ?`, sample.ID).Scan(&status); err != nil { t.Fatal(err) }
    if status != string(domain.SampleCollected) { t.Fatalf("failed handoff left status %q", status) }
    if _, err := f.store.DB().Exec(`DROP TRIGGER fail_sampling_handoff_audit`); err != nil { t.Fatal(err) }
    if _, err := f.sampling.Handoff(context.Background(), f.field, sampling.HandoffCommand{SampleID: sample.ID, ToUserID: f.analyst.UserID, OccurredAt: time.Now().UTC(), RequestID: "private-handoff-retry"}); err != nil { t.Fatal(err) }
    if err := f.store.DB().QueryRow(`SELECT status FROM samples WHERE id = ?`, sample.ID).Scan(&status); err != nil { t.Fatal(err) }
    if status != string(domain.SampleInTransit) { t.Fatalf("retry status = %q", status) }
}
