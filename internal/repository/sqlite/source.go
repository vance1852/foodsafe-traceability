package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vance1852/foodsafe-traceability/internal/domain"
)

func InsertFoodFacility(ctx context.Context, db DBTX, source domain.FoodFacility) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO food_facilities(
			id, organization_id, name, kind, timezone, active,
			version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		source.ID,
		source.OrganizationID,
		source.Name,
		string(source.Kind),
		source.Timezone,
		boolInt(source.Active),
		source.Version,
		formatTime(source.CreatedAt),
		formatTime(source.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert food facility: %w", err)
	}
	return nil
}

func scanFoodFacility(scanner interface{ Scan(...any) error }) (domain.FoodFacility, error) {
	var source domain.FoodFacility
	var kind, created, updated string
	var active int
	err := scanner.Scan(
		&source.ID,
		&source.OrganizationID,
		&source.Name,
		&kind,
		&source.Timezone,
		&active,
		&source.Version,
		&created,
		&updated,
	)
	if err != nil {
		return domain.FoodFacility{}, err
	}
	source.Kind = domain.FacilityKind(kind)
	source.Active = active == 1
	var parseErr error
	if source.CreatedAt, parseErr = parseTime(created); parseErr != nil {
		return domain.FoodFacility{}, parseErr
	}
	if source.UpdatedAt, parseErr = parseTime(updated); parseErr != nil {
		return domain.FoodFacility{}, parseErr
	}
	return source, nil
}

const selectSourceColumns = `id, organization_id, name, kind, timezone, active, version, created_at, updated_at`

func (s *Store) FoodFacility(ctx context.Context, db DBTX, organizationID, sourceID string) (domain.FoodFacility, error) {
	source, err := scanFoodFacility(db.QueryRowContext(ctx, "SELECT "+selectSourceColumns+" FROM food_facilities WHERE organization_id = ? AND id = ?", organizationID, sourceID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FoodFacility{}, &domain.NotFoundError{Resource: "food facility", ID: sourceID}
	}
	if err != nil {
		return domain.FoodFacility{}, fmt.Errorf("select food facility: %w", err)
	}
	return source, nil
}

func (s *Store) ListFoodFacilities(ctx context.Context, organizationID string, activeOnly bool, page domain.PageRequest) (domain.Page[domain.FoodFacility], error) {
	page = page.Normalized()
	query := "SELECT " + selectSourceColumns + " FROM food_facilities WHERE organization_id = ?"
	args := []any{organizationID}
	if activeOnly {
		query += " AND active = 1"
	}
	if page.Cursor != "" {
		query += " AND id > ?"
		args = append(args, page.Cursor)
	}
	query += " ORDER BY id ASC LIMIT ?"
	args = append(args, page.Limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return domain.Page[domain.FoodFacility]{}, fmt.Errorf("query food facilities: %w", err)
	}
	defer rows.Close()
	items := make([]domain.FoodFacility, 0, page.Limit)
	for rows.Next() {
		source, err := scanFoodFacility(rows)
		if err != nil {
			return domain.Page[domain.FoodFacility]{}, fmt.Errorf("scan food facility: %w", err)
		}
		items = append(items, source)
	}
	if err := rows.Err(); err != nil {
		return domain.Page[domain.FoodFacility]{}, fmt.Errorf("iterate food facilities: %w", err)
	}
	result := domain.Page[domain.FoodFacility]{Items: items}
	if len(items) > page.Limit {
		result.NextCursor = items[page.Limit-1].ID
		result.Items = items[:page.Limit]
	}
	return result, nil
}

func InsertProductionZone(ctx context.Context, db DBTX, zone domain.ProductionZone) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO production_zones(
			id, facility_id, organization_id, name, level, area_square_meters,
			active, version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		zone.ID,
		zone.FacilityID,
		zone.OrganizationID,
		zone.Name,
		string(zone.Level),
		zone.AreaSquareMeters,
		boolInt(zone.Active),
		zone.Version,
		formatTime(zone.CreatedAt),
		formatTime(zone.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert production zone: %w", err)
	}
	return nil
}

func (s *Store) ProductionZone(ctx context.Context, db DBTX, organizationID, zoneID string) (domain.ProductionZone, error) {
	var zone domain.ProductionZone
	var level, created, updated string
	var active int
	err := db.QueryRowContext(ctx, `
		SELECT id, facility_id, organization_id, name, level, area_square_meters,
		       active, version, created_at, updated_at
		FROM production_zones
		WHERE organization_id = ? AND id = ?`, organizationID, zoneID).Scan(
		&zone.ID,
		&zone.FacilityID,
		&zone.OrganizationID,
		&zone.Name,
		&level,
		&zone.AreaSquareMeters,
		&active,
		&zone.Version,
		&created,
		&updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProductionZone{}, &domain.NotFoundError{Resource: "production zone", ID: zoneID}
	}
	if err != nil {
		return domain.ProductionZone{}, fmt.Errorf("select production zone: %w", err)
	}
	zone.Level = domain.ProductionZoneLevel(level)
	zone.Active = active == 1
	if zone.CreatedAt, err = parseTime(created); err != nil {
		return domain.ProductionZone{}, err
	}
	if zone.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.ProductionZone{}, err
	}
	return zone, nil
}

func InsertInspectionStation(ctx context.Context, db DBTX, station domain.InspectionStation) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO inspection_stations(
			id, facility_id, zone_id, organization_id, code, name,
			latitude, longitude, active, version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		station.ID,
		station.FacilityID,
		station.ZoneID,
		station.OrganizationID,
		strings.ToUpper(station.Code),
		station.Name,
		station.Latitude,
		station.Longitude,
		boolInt(station.Active),
		station.Version,
		formatTime(station.CreatedAt),
		formatTime(station.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert inspection station: %w", err)
	}
	return nil
}

func (s *Store) InspectionStation(ctx context.Context, db DBTX, organizationID, stationID string) (domain.InspectionStation, error) {
	var station domain.InspectionStation
	var active int
	var created, updated string
	err := db.QueryRowContext(ctx, `
		SELECT id, facility_id, zone_id, organization_id, code, name,
		       latitude, longitude, active, version, created_at, updated_at
		FROM inspection_stations
		WHERE organization_id = ? AND id = ?`, organizationID, stationID).Scan(
		&station.ID,
		&station.FacilityID,
		&station.ZoneID,
		&station.OrganizationID,
		&station.Code,
		&station.Name,
		&station.Latitude,
		&station.Longitude,
		&active,
		&station.Version,
		&created,
		&updated,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.InspectionStation{}, &domain.NotFoundError{Resource: "inspection station", ID: stationID}
	}
	if err != nil {
		return domain.InspectionStation{}, fmt.Errorf("select inspection station: %w", err)
	}
	station.Active = active == 1
	if station.CreatedAt, err = parseTime(created); err != nil {
		return domain.InspectionStation{}, err
	}
	if station.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.InspectionStation{}, err
	}
	return station, nil
}

func (s *Store) ListStations(ctx context.Context, organizationID, sourceID string, activeOnly bool) ([]domain.InspectionStation, error) {
	query := `
		SELECT id, facility_id, zone_id, organization_id, code, name,
		       latitude, longitude, active, version, created_at, updated_at
		FROM inspection_stations
		WHERE organization_id = ? AND facility_id = ?`
	if activeOnly {
		query += " AND active = 1"
	}
	query += " ORDER BY code ASC"
	rows, err := s.db.QueryContext(ctx, query, organizationID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("query inspection stations: %w", err)
	}
	defer rows.Close()
	stations := make([]domain.InspectionStation, 0)
	for rows.Next() {
		var station domain.InspectionStation
		var active int
		var created, updated string
		if err := rows.Scan(
			&station.ID,
			&station.FacilityID,
			&station.ZoneID,
			&station.OrganizationID,
			&station.Code,
			&station.Name,
			&station.Latitude,
			&station.Longitude,
			&active,
			&station.Version,
			&created,
			&updated,
		); err != nil {
			return nil, fmt.Errorf("scan inspection station: %w", err)
		}
		station.Active = active == 1
		station.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		station.UpdatedAt, err = parseTime(updated)
		if err != nil {
			return nil, err
		}
		stations = append(stations, station)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inspection stations: %w", err)
	}
	return stations, nil
}

func (s *Store) UpdateSourceActive(ctx context.Context, organizationID, sourceID string, expectedVersion int64, active bool, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE food_facilities
		SET active = ?, version = version + 1, updated_at = ?
		WHERE organization_id = ? AND id = ? AND version = ?`,
		boolInt(active), formatTime(now), organizationID, sourceID, expectedVersion)
	if err != nil {
		return fmt.Errorf("update source active state: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read source update count: %w", err)
	}
	if changed != 1 {
		return &domain.ConflictError{Resource: "food facility", Key: sourceID, Cause: domain.ErrConflict}
	}
	return nil
}
