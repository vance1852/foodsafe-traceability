package source

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vance1852/foodsafe-traceability/internal/audit"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
	repository "github.com/vance1852/foodsafe-traceability/internal/repository/sqlite"
)

type Service struct {
	store *repository.Store
	clock func() time.Time
}

type RegisterSourceCommand struct {
	Name      string              `json:"name"`
	Kind      domain.FacilityKind `json:"kind"`
	Timezone  string              `json:"timezone"`
	RequestID string              `json:"-"`
}

type RegisterZoneCommand struct {
	SourceID         string                     `json:"source_id"`
	Name             string                     `json:"name"`
	Level            domain.ProductionZoneLevel `json:"level"`
	AreaSquareMeters int64                      `json:"area_square_meters"`
	RequestID        string                     `json:"-"`
}

type RegisterStationCommand struct {
	SourceID  string  `json:"source_id"`
	ZoneID    string  `json:"zone_id"`
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RequestID string  `json:"-"`
}

func NewService(store *repository.Store) *Service {
	return &Service{store: store, clock: time.Now}
}

func (s *Service) RegisterFoodFacility(ctx context.Context, actor domain.Actor, command RegisterSourceCommand) (domain.FoodFacility, error) {
	if !actor.CanSupervise() {
		return domain.FoodFacility{}, domain.ErrForbidden
	}
	now := s.clock().UTC()
	source := domain.FoodFacility{
		ID:             uuid.NewString(),
		OrganizationID: actor.OrganizationID,
		Name:           strings.TrimSpace(command.Name),
		Kind:           command.Kind,
		Timezone:       strings.TrimSpace(command.Timezone),
		Active:         true,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := source.Validate(); err != nil {
		return domain.FoodFacility{}, err
	}
	if strings.TrimSpace(command.RequestID) == "" {
		return domain.FoodFacility{}, domain.NewValidationError("register food facility", domain.FieldViolation{Field: "request_id", Rule: "is required"})
	}
	err := s.store.WithTx(ctx, nil, func(tx *sql.Tx) error {
		if err := repository.InsertFoodFacility(ctx, tx, source); err != nil {
			return err
		}
		metadata, _ := json.Marshal(map[string]any{"kind": source.Kind, "timezone": source.Timezone})
		return audit.Insert(ctx, tx, domain.AuditEvent{
			ID:             uuid.NewString(),
			OrganizationID: actor.OrganizationID,
			ActorUserID:    actor.UserID,
			RequestID:      command.RequestID,
			Action:         "water_source.register",
			ObjectType:     "water_source",
			ObjectID:       source.ID,
			Outcome:        "success",
			Metadata:       string(metadata),
			OccurredAt:     now,
		})
	})
	if err != nil {
		return domain.FoodFacility{}, fmt.Errorf("register food facility: %w", err)
	}
	return source, nil
}

func (s *Service) RegisterZone(ctx context.Context, actor domain.Actor, command RegisterZoneCommand) (domain.ProductionZone, error) {
	if !actor.CanSupervise() {
		return domain.ProductionZone{}, domain.ErrForbidden
	}
	now := s.clock().UTC()
	zone := domain.ProductionZone{
		ID:               uuid.NewString(),
		SourceID:         command.SourceID,
		OrganizationID:   actor.OrganizationID,
		Name:             strings.TrimSpace(command.Name),
		Level:            command.Level,
		AreaSquareMeters: command.AreaSquareMeters,
		Active:           true,
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := zone.Validate(); err != nil {
		return domain.ProductionZone{}, err
	}
	err := s.store.WithTx(ctx, nil, func(tx *sql.Tx) error {
		source, err := s.store.FoodFacility(ctx, tx, actor.OrganizationID, command.SourceID)
		if err != nil {
			return err
		}
		if !source.Active {
			return &domain.ConflictError{Resource: "food facility", Key: source.ID, Cause: errors.New("inactive sources cannot receive zones")}
		}
		if err := repository.InsertProductionZone(ctx, tx, zone); err != nil {
			return err
		}
		return audit.Insert(ctx, tx, domain.AuditEvent{
			ID: uuid.NewString(), OrganizationID: actor.OrganizationID, ActorUserID: actor.UserID,
			RequestID: command.RequestID, Action: "protection_zone.register", ObjectType: "protection_zone",
			ObjectID: zone.ID, Outcome: "success", Metadata: fmt.Sprintf(`{"source_id":%q,"level":%q}`, zone.SourceID, zone.Level), OccurredAt: now,
		})
	})
	if err != nil {
		return domain.ProductionZone{}, fmt.Errorf("register production zone: %w", err)
	}
	return zone, nil
}

func (s *Service) RegisterStation(ctx context.Context, actor domain.Actor, command RegisterStationCommand) (domain.InspectionStation, error) {
	if !actor.CanSupervise() {
		return domain.InspectionStation{}, domain.ErrForbidden
	}
	now := s.clock().UTC()
	station := domain.InspectionStation{
		ID: uuid.NewString(), SourceID: command.SourceID, ZoneID: command.ZoneID,
		OrganizationID: actor.OrganizationID, Code: strings.ToUpper(strings.TrimSpace(command.Code)),
		Name: strings.TrimSpace(command.Name), Latitude: command.Latitude, Longitude: command.Longitude,
		Active: true, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := station.Validate(); err != nil {
		return domain.InspectionStation{}, err
	}
	err := s.store.WithTx(ctx, nil, func(tx *sql.Tx) error {
		source, err := s.store.FoodFacility(ctx, tx, actor.OrganizationID, station.SourceID)
		if err != nil {
			return err
		}
		zone, err := s.store.ProductionZone(ctx, tx, actor.OrganizationID, station.ZoneID)
		if err != nil {
			return err
		}
		if zone.SourceID != source.ID {
			return &domain.ConflictError{Resource: "production zone", Key: zone.ID, Cause: errors.New("zone belongs to a different food facility")}
		}
		if !source.Active || !zone.Active {
			return &domain.ConflictError{Resource: "inspection station", Key: station.Code, Cause: errors.New("source and zone must be active")}
		}
		if err := repository.InsertInspectionStation(ctx, tx, station); err != nil {
			return err
		}
		return audit.Insert(ctx, tx, domain.AuditEvent{
			ID: uuid.NewString(), OrganizationID: actor.OrganizationID, ActorUserID: actor.UserID,
			RequestID: command.RequestID, Action: "monitoring_station.register", ObjectType: "monitoring_station",
			ObjectID: station.ID, Outcome: "success", Metadata: fmt.Sprintf(`{"source_id":%q,"zone_id":%q}`, station.SourceID, station.ZoneID), OccurredAt: now,
		})
	})
	if err != nil {
		return domain.InspectionStation{}, fmt.Errorf("register inspection station: %w", err)
	}
	return station, nil
}

func (s *Service) ListSources(ctx context.Context, actor domain.Actor, activeOnly bool, page domain.PageRequest) (domain.Page[domain.FoodFacility], error) {
	if err := actor.Validate(); err != nil {
		return domain.Page[domain.FoodFacility]{}, err
	}
	result, err := s.store.ListFoodFacilities(ctx, actor.OrganizationID, activeOnly, page)
	if err != nil {
		return domain.Page[domain.FoodFacility]{}, fmt.Errorf("list food facilities: %w", err)
	}
	return result, nil
}

func (s *Service) ListStations(ctx context.Context, actor domain.Actor, sourceID string) ([]domain.InspectionStation, error) {
	if _, err := s.store.FoodFacility(ctx, s.store.DB(), actor.OrganizationID, sourceID); err != nil {
		return nil, err
	}
	stations, err := s.store.ListStations(ctx, actor.OrganizationID, sourceID, true)
	if err != nil {
		return nil, fmt.Errorf("list inspection stations: %w", err)
	}
	return stations, nil
}
