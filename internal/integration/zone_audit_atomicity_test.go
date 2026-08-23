package integration_test

import (
	"context"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/source"
	"testing"
)

func TestZoneRegistrationAuditFailureRollsBackZone(t *testing.T) {
	f := newFixture(t)
	g := f.createSourceGraph(t)
	ctx := context.Background()
	if _, err := f.store.DB().ExecContext(ctx, `CREATE TRIGGER reject_zone_audit BEFORE INSERT ON audit_events WHEN NEW.action='protection_zone.register' BEGIN SELECT RAISE(FAIL,'zone audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.sources.RegisterZone(ctx, f.supervisor, source.RegisterZoneCommand{FacilityID: g.source.ID, Name: "Cold Zone", Level: domain.ZoneSecondary, AreaSquareMeters: 800, RequestID: "zone-failed"})
	if err == nil {
		t.Fatal("zone registration succeeded while audit was rejected")
	}
	var zones, audits int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM production_zones WHERE facility_id=? AND name='Cold Zone'`, g.source.ID).Scan(&zones); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='zone-failed'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if zones != 0 || audits != 0 {
		t.Fatalf("failed zone left zones=%d audits=%d", zones, audits)
	}
	if _, err := f.store.DB().ExecContext(ctx, `DROP TRIGGER reject_zone_audit`); err != nil {
		t.Fatal(err)
	}
	zone, err := f.sources.RegisterZone(ctx, f.supervisor, source.RegisterZoneCommand{FacilityID: g.source.ID, Name: "Cold Zone", Level: domain.ZoneSecondary, AreaSquareMeters: 800, RequestID: "zone-retry"})
	if err != nil {
		t.Fatalf("retry zone: %v", err)
	}
	if zone.Level != domain.ZoneSecondary {
		t.Fatalf("retry zone level=%s", zone.Level)
	}
	var count int
	if err := f.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE request_id='zone-retry'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry audits=%d", count)
	}
}
