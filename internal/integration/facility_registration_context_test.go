package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
	"github.com/vance1852/foodsafe-traceability/internal/source"
)

func TestCancelledFacilityRegistrationLeavesNoPersistentState(t *testing.T) {
	f := newFixture(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.sources.RegisterFoodFacility(cancelled, f.supervisor, source.RegisterSourceCommand{
		Name: "Abandoned Cold Storage", Kind: domain.FacilityColdStorage,
		Timezone: "UTC", RequestID: "cancelled-facility-registration",
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled registration error = %v, want context cancellation", err)
	}
	var cancelledFacilities, cancelledAudits int
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM food_facilities WHERE name = 'Abandoned Cold Storage'`).Scan(&cancelledFacilities); err != nil {
		t.Fatalf("count cancelled facilities: %v", err)
	}
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE request_id = 'cancelled-facility-registration'`).Scan(&cancelledAudits); err != nil {
		t.Fatalf("count cancelled registration audits: %v", err)
	}
	if cancelledFacilities != 0 || cancelledAudits != 0 {
		t.Errorf("cancelled registration persisted facilities=%d audits=%d", cancelledFacilities, cancelledAudits)
	}

	created, err := f.sources.RegisterFoodFacility(context.Background(), f.supervisor, source.RegisterSourceCommand{
		Name: "Operating Cold Storage", Kind: domain.FacilityColdStorage,
		Timezone: "UTC", RequestID: "live-facility-registration",
	})
	if err != nil {
		t.Fatalf("live registration: %v", err)
	}
	if created.Name != "Operating Cold Storage" || created.OrganizationID != f.supervisor.OrganizationID {
		t.Fatalf("live facility = %#v", created)
	}
	var liveAudits int
	if err := f.store.DB().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE request_id = 'live-facility-registration'`).Scan(&liveAudits); err != nil {
		t.Fatalf("count live registration audits: %v", err)
	}
	if liveAudits != 1 {
		t.Fatalf("live registration audits = %d, want 1", liveAudits)
	}
}
