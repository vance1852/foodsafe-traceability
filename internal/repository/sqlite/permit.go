package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vance1852/foodsafe-traceability/internal/domain"
)

func InsertPermit(ctx context.Context, db DBTX, permit domain.Permit) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO permits(
			id, organization_id, facility_id, holder_name, reference,
			valid_from, valid_until, daily_volume_limit_liters,
			status, version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		permit.ID, permit.OrganizationID, permit.FacilityID, permit.HolderName, permit.Reference,
		formatTime(permit.ValidFrom), formatTime(permit.ValidUntil), permit.DailyVolumeLimitLiters,
		string(permit.Status), permit.Version, formatTime(permit.CreatedAt), formatTime(permit.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("insert permit: %w", err)
	}
	return nil
}

func scanPermit(scanner interface{ Scan(...any) error }) (domain.Permit, error) {
	var permit domain.Permit
	var validFrom, validUntil, status, created, updated string
	err := scanner.Scan(
		&permit.ID, &permit.OrganizationID, &permit.FacilityID, &permit.HolderName,
		&permit.Reference, &validFrom, &validUntil, &permit.DailyVolumeLimitLiters,
		&status, &permit.Version, &created, &updated,
	)
	if err != nil {
		return domain.Permit{}, err
	}
	permit.Status = domain.PermitStatus(status)
	var parseErr error
	if permit.ValidFrom, parseErr = parseTime(validFrom); parseErr != nil {
		return domain.Permit{}, parseErr
	}
	if permit.ValidUntil, parseErr = parseTime(validUntil); parseErr != nil {
		return domain.Permit{}, parseErr
	}
	if permit.CreatedAt, parseErr = parseTime(created); parseErr != nil {
		return domain.Permit{}, parseErr
	}
	if permit.UpdatedAt, parseErr = parseTime(updated); parseErr != nil {
		return domain.Permit{}, parseErr
	}
	return permit, nil
}

const selectPermit = `
	id, organization_id, facility_id, holder_name, reference,
	valid_from, valid_until, daily_volume_limit_liters,
	status, version, created_at, updated_at`

func (s *Store) Permit(ctx context.Context, db DBTX, organizationID, permitID string) (domain.Permit, error) {
	permit, err := scanPermit(db.QueryRowContext(ctx, "SELECT "+selectPermit+" FROM permits WHERE organization_id = ? AND id = ?", organizationID, permitID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Permit{}, &domain.NotFoundError{Resource: "permit", ID: permitID}
	}
	if err != nil {
		return domain.Permit{}, fmt.Errorf("select permit: %w", err)
	}
	return permit, nil
}

func (s *Store) TransitionPermit(ctx context.Context, tx *sql.Tx, permit domain.Permit, to domain.PermitStatus, now time.Time) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE permits
		SET status = ?, version = version + 1, updated_at = ?
		WHERE organization_id = ? AND id = ? AND status = ? AND version = ?`,
		string(to), formatTime(now), permit.OrganizationID, permit.ID, string(permit.Status), permit.Version)
	if err != nil {
		return fmt.Errorf("transition permit: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read permit transition count: %w", err)
	}
	if changed != 1 {
		return &domain.ConflictError{Resource: "permit", Key: permit.ID, Cause: domain.ErrConflict}
	}
	return nil
}

// SuspendPermit transitions a permit into the suspended state and enqueues the
// permit.suspended outbox event inside its own transaction. Deprecated for the
// permit service: prefer SuspendPermitTx, which leaves the transaction open so
// the caller can append audit (and other) writes atomically.
func (s *Store) SuspendPermit(ctx context.Context, organizationID, permitID, reason string, now time.Time) (domain.Permit, error) {
	var permit domain.Permit
	err := s.WithTx(ctx, nil, func(tx *sql.Tx) error {
		var err error
		permit, err = s.SuspendPermitTx(ctx, tx, organizationID, permitID, reason, now)
		return err
	})
	if err != nil {
		return domain.Permit{}, fmt.Errorf("persist permit suspension: %w", err)
	}
	return permit, nil
}

// SuspendPermitTx performs the permit transition and outbox enqueue for a
// suspension on the caller's transaction without committing, so the caller can
// add audit records (and any other dependent writes) and commit them together.
func (s *Store) SuspendPermitTx(ctx context.Context, tx *sql.Tx, organizationID, permitID, reason string, now time.Time) (domain.Permit, error) {
	permit, err := s.Permit(ctx, tx, organizationID, permitID)
	if err != nil {
		return domain.Permit{}, err
	}
	if err := permit.CanTransition(domain.PermitSuspended, now); err != nil {
		return domain.Permit{}, err
	}
	if err := s.TransitionPermit(ctx, tx, permit, domain.PermitSuspended, now); err != nil {
		return domain.Permit{}, err
	}
	payload, err := json.Marshal(map[string]any{"permit_id": permit.ID, "reason": reason})
	if err != nil {
		return domain.Permit{}, fmt.Errorf("encode permit suspension event: %w", err)
	}
	if err := InsertOutboxEvent(ctx, tx, domain.OutboxEvent{
		ID: uuid.NewString(), OrganizationID: organizationID, Topic: "permit.suspended",
		AggregateType: "permit", AggregateID: permit.ID, IdempotencyKey: "suspend:" + permit.ID + ":" + fmt.Sprint(permit.Version),
		Payload: payload, Status: domain.OutboxPending, MaxAttempts: 5, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return domain.Permit{}, err
	}
	return permit, nil
}

func (s *Store) DailyShipmentReleaseVolume(ctx context.Context, db DBTX, permitID string, dayStart, dayEnd time.Time) (int64, error) {
	var total int64
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(volume_liters), 0)
		FROM shipment_release_events
		WHERE permit_id = ? AND occurred_at >= ? AND occurred_at < ?`,
		permitID, formatTime(dayStart), formatTime(dayEnd)).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum daily shipment release volume: %w", err)
	}
	return total, nil
}

func InsertShipmentReleaseEvent(ctx context.Context, db DBTX, event domain.ShipmentReleaseEvent) (bool, error) {
	result, err := db.ExecContext(ctx, `
		INSERT INTO shipment_release_events(
			id, organization_id, permit_id, idempotency_key,
			volume_liters, occurred_at, reported_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(organization_id, permit_id, idempotency_key) DO NOTHING`,
		event.ID, event.OrganizationID, event.PermitID, event.IdempotencyKey,
		event.VolumeLiters, formatTime(event.OccurredAt), event.ReportedBy, formatTime(event.CreatedAt),
	)
	if err != nil {
		return false, fmt.Errorf("insert shipment release event: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read shipment release insert count: %w", err)
	}
	return changed == 1, nil
}

func (s *Store) ExistingShipmentReleaseByKey(ctx context.Context, db DBTX, organizationID, permitID, key string) (domain.ShipmentReleaseEvent, error) {
	var event domain.ShipmentReleaseEvent
	var occurred, created string
	err := db.QueryRowContext(ctx, `
		SELECT id, organization_id, permit_id, idempotency_key,
		       volume_liters, occurred_at, reported_by, created_at
		FROM shipment_release_events
		WHERE organization_id = ? AND permit_id = ? AND idempotency_key = ?`,
		organizationID, permitID, key).Scan(
		&event.ID, &event.OrganizationID, &event.PermitID, &event.IdempotencyKey,
		&event.VolumeLiters, &occurred, &event.ReportedBy, &created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ShipmentReleaseEvent{}, &domain.NotFoundError{Resource: "shipment release event", ID: key}
	}
	if err != nil {
		return domain.ShipmentReleaseEvent{}, fmt.Errorf("select shipment release by idempotency key: %w", err)
	}
	if event.OccurredAt, err = parseTime(occurred); err != nil {
		return domain.ShipmentReleaseEvent{}, err
	}
	if event.CreatedAt, err = parseTime(created); err != nil {
		return domain.ShipmentReleaseEvent{}, err
	}
	return event, nil
}
