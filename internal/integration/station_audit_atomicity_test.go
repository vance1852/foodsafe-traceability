package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/source"
	"testing"
)

func TestStationRegistrationAuditFailureRollsBackStation(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	ctx := context.Background()
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_station_audit BEFORE INSERT ON audit_events WHEN NEW.action='monitoring_station.register' BEGIN SELECT RAISE(FAIL,'station audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.sources.RegisterStation(ctx, f.supervisor, source.RegisterStationCommand{FacilityID: g.source.ID, ZoneID: g.zone.ID, Code: "COLD-2", Name: "Cold inlet", Latitude: 31.3, Longitude: 121.5, RequestID: "station-failed"})
	if err == nil {
		t.Fatal("station registration succeeded while audit was rejected")
	}
	var stations, audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM inspection_stations WHERE code='COLD-2'`).Scan(&stations); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='station-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if stations != 0 || audits != 0 {
		t.Fatalf("failed station left stations=%d audits=%d", stations, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_station_audit`); err != nil {
		t.Fatal(err)
	}
	station, err := f.sources.RegisterStation(ctx, f.supervisor, source.RegisterStationCommand{FacilityID: g.source.ID, ZoneID: g.zone.ID, Code: "COLD-2", Name: "Cold inlet", Latitude: 31.3, Longitude: 121.5, RequestID: "station-retry"})
	if err != nil {
		t.Fatalf("retry station: %v", err)
	}
	if station.Code != "COLD-2" {
		t.Fatalf("retry code=%s", station.Code)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='station-retry'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("retry audits=%d", audits)
	}
}
