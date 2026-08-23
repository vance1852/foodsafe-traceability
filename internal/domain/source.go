package domain

import (
	"strings"
	"time"
)

type FacilityKind string

const (
	FacilityProcessing   FacilityKind = "processing_plant"
	FacilityDistribution FacilityKind = "distribution_center"
	FacilityColdStorage  FacilityKind = "cold_storage"
)

type FoodFacility struct {
	ID             string
	OrganizationID string
	Name           string
	Kind           FacilityKind
	Timezone       string
	Active         bool
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s FoodFacility) Validate() error {
	var violations []FieldViolation
	if strings.TrimSpace(s.OrganizationID) == "" {
		violations = append(violations, FieldViolation{Field: "organization_id", Rule: "is required"})
	}
	if len(strings.TrimSpace(s.Name)) < 3 {
		violations = append(violations, FieldViolation{Field: "name", Rule: "must contain at least 3 characters"})
	}
	switch s.Kind {
	case FacilityProcessing, FacilityDistribution, FacilityColdStorage:
	default:
		violations = append(violations, FieldViolation{Field: "kind", Rule: "is unsupported"})
	}
	if strings.TrimSpace(s.Timezone) == "" {
		violations = append(violations, FieldViolation{Field: "timezone", Rule: "is required"})
	} else if _, err := time.LoadLocation(s.Timezone); err != nil {
		violations = append(violations, FieldViolation{Field: "timezone", Rule: "must be an IANA timezone"})
	}
	if len(violations) > 0 {
		return NewValidationError("validate food facility", violations...)
	}
	return nil
}

type ProductionZoneLevel string

const (
	ZonePrimary   ProductionZoneLevel = "primary"
	ZoneSecondary ProductionZoneLevel = "secondary"
	ZoneBuffer    ProductionZoneLevel = "buffer"
)

type ProductionZone struct {
	ID               string
	FacilityID       string
	OrganizationID   string
	Name             string
	Level            ProductionZoneLevel
	AreaSquareMeters int64
	Active           bool
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (z ProductionZone) Validate() error {
	var violations []FieldViolation
	if z.FacilityID == "" {
		violations = append(violations, FieldViolation{Field: "facility_id", Rule: "is required"})
	}
	if z.OrganizationID == "" {
		violations = append(violations, FieldViolation{Field: "organization_id", Rule: "is required"})
	}
	if strings.TrimSpace(z.Name) == "" {
		violations = append(violations, FieldViolation{Field: "name", Rule: "is required"})
	}
	switch z.Level {
	case ZonePrimary, ZoneSecondary, ZoneBuffer:
	default:
		violations = append(violations, FieldViolation{Field: "level", Rule: "is unsupported"})
	}
	if z.AreaSquareMeters <= 0 {
		violations = append(violations, FieldViolation{Field: "area_square_meters", Rule: "must be positive"})
	}
	if len(violations) > 0 {
		return NewValidationError("validate production zone", violations...)
	}
	return nil
}

type InspectionStation struct {
	ID             string
	FacilityID     string
	ZoneID         string
	OrganizationID string
	Code           string
	Name           string
	Latitude       float64
	Longitude      float64
	Active         bool
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s InspectionStation) Validate() error {
	var violations []FieldViolation
	if s.FacilityID == "" || s.ZoneID == "" || s.OrganizationID == "" {
		violations = append(violations, FieldViolation{Field: "ownership", Rule: "source, zone and organization are required"})
	}
	if strings.TrimSpace(s.Code) == "" {
		violations = append(violations, FieldViolation{Field: "code", Rule: "is required"})
	}
	if strings.TrimSpace(s.Name) == "" {
		violations = append(violations, FieldViolation{Field: "name", Rule: "is required"})
	}
	if s.Latitude < -90 || s.Latitude > 90 {
		violations = append(violations, FieldViolation{Field: "latitude", Rule: "must be between -90 and 90"})
	}
	if s.Longitude < -180 || s.Longitude > 180 {
		violations = append(violations, FieldViolation{Field: "longitude", Rule: "must be between -180 and 180"})
	}
	if len(violations) > 0 {
		return NewValidationError("validate inspection station", violations...)
	}
	return nil
}
