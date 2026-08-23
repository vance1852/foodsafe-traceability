package telemetry

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	repository "github.com/vance1852/foodsafe-traceability/internal/repository/sqlite"
)

// seedTelemetryGraph inserts the organization, facility, zone and station rows that
// telemetry ingestion depends on.
func seedTelemetryGraph(t *testing.T, store *repository.Store, stationID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	stmts := []string{
		`INSERT INTO organizations(id, name, created_at) VALUES ('org', 'Authority', ?)`,
		`INSERT INTO food_facilities(id, organization_id, name, kind, timezone, active, version, created_at, updated_at) VALUES ('src', 'org', 'Foods Plant', 'processing_plant', 'UTC', 1, 1, ?, ?)`,
		`INSERT INTO production_zones(id, facility_id, organization_id, name, level, area_square_meters, active, version, created_at, updated_at) VALUES ('zone', 'src', 'org', 'Primary', 'primary', 100, 1, 1, ?, ?)`,
		fmt.Sprintf(`INSERT INTO inspection_stations(id, facility_id, zone_id, organization_id, code, name, latitude, longitude, active, version, created_at, updated_at) VALUES (?, 'src', 'zone', 'org', 'S1', 'Station', 0, 0, 1, 1, ?, ?)`),
	}
	args := [][]any{
		{now.Format(time.RFC3339Nano)},
		{now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)},
		{now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)},
		{stationID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)},
	}
	for i, stmt := range stmts {
		if _, err := store.DB().ExecContext(ctx, stmt, args[i]...); err != nil {
			t.Fatalf("seed statement %d: %v", i, err)
		}
	}
}

// TestIngestRollsBackReadingAndJobWhenAuditStoreUnavailable simulates an audit-store
// outage by dropping audit_events, then confirms a retry with the same external_id can
// still create the reading exactly once once audit storage recovers.
func TestIngestRollsBackReadingAndJobWhenAuditStoreUnavailable(t *testing.T) {
	store, err := repository.Open(context.Background(), filepath.Join(t.TempDir(), "ingest.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	seedTelemetryGraph(t, store, "station")

	// Simulate audit storage being unavailable: drop the audit table so inserts fail.
	if _, err := store.DB().ExecContext(ctx, `DROP TABLE audit_events`); err != nil {
		t.Fatalf("drop audit_events: %v", err)
	}

	service := NewService(store)
	service.clock = func() time.Time { return now }
	actor := domain.Actor{UserID: "field", OrganizationID: "org", Role: domain.RoleFieldOperator}
	command := IngestCommand{
		StationID: "station", ExternalID: "sensor-reading-1", Parameter: "temperature",
		Value: 90, Unit: "C", Threshold: 75, ObservedAt: now, RequestID: "telemetry-1",
	}

	if _, _, err := service.Ingest(ctx, actor, command); err == nil {
		t.Fatal("Ingest() expected error when audit store unavailable, got nil")
	}

	// Nothing should have persisted: the audit failure rolled back reading and job.
	var readings, jobs int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM telemetry_readings WHERE station_id = 'station'`).Scan(&readings); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM alert_jobs`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if readings != 0 || jobs != 0 {
		t.Fatalf("audit failure should roll back reading/job: readings=%d jobs=%d", readings, jobs)
	}

	// Audit storage recovers; the same external_id must now create the reading once.
	if _, err := store.DB().ExecContext(ctx, `CREATE TABLE audit_events (
		id TEXT PRIMARY KEY,
		organization_id TEXT NOT NULL REFERENCES organizations(id),
		actor_user_id TEXT NOT NULL,
		request_id TEXT NOT NULL,
		action TEXT NOT NULL,
		object_type TEXT NOT NULL,
		object_id TEXT NOT NULL,
		outcome TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		occurred_at TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("recreate audit_events: %v", err)
	}

	reading, created, err := service.Ingest(ctx, actor, command)
	if err != nil || !created {
		t.Fatalf("retry ingest created=%v err=%v", created, err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM telemetry_readings WHERE station_id = 'station'`).Scan(&readings); err != nil {
		t.Fatal(err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM alert_jobs WHERE reading_id = ?`, reading.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action = 'telemetry.ingest'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if readings != 1 || jobs != 1 || audits != 1 {
		t.Fatalf("after recovery readings=%d jobs=%d audits=%d", readings, jobs, audits)
	}

	// A second retry with the same external_id is idempotent and reports no new creation.
	_, created, err = service.Ingest(ctx, actor, command)
	if err != nil || created {
		t.Fatalf("duplicate ingest created=%v err=%v", created, err)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM telemetry_readings WHERE station_id = 'station'`).Scan(&readings); err != nil {
		t.Fatal(err)
	}
	if readings != 1 {
		t.Fatalf("duplicate ingest should not add readings: %d", readings)
	}
}
