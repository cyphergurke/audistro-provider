package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AnnouncementRow struct {
	AssetID         string
	LastAnnouncedAt time.Time
	ExpiresAt       time.Time
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func UpsertAnnouncementStatus(ctx context.Context, db *sql.DB, assetID string, lastAnnouncedAt, expiresAt time.Time, status string, now time.Time) error {
	lastAnnounced := ""
	if !lastAnnouncedAt.IsZero() {
		lastAnnounced = lastAnnouncedAt.UTC().Format(time.RFC3339)
	}
	expires := ""
	if !expiresAt.IsZero() {
		expires = expiresAt.UTC().Format(time.RFC3339)
	}
	nowStr := now.UTC().Format(time.RFC3339)

	query := `INSERT INTO announcements (
		asset_id, last_announced_at, expires_at, status, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(asset_id) DO UPDATE SET
		last_announced_at=excluded.last_announced_at,
		expires_at=excluded.expires_at,
		status=excluded.status,
		updated_at=excluded.updated_at`

	if _, err := db.ExecContext(ctx, query, assetID, lastAnnounced, expires, status, nowStr, nowStr); err != nil {
		return fmt.Errorf("upsert announcement status for asset %q: %w", assetID, err)
	}
	return nil
}

func GetAnnouncement(ctx context.Context, db *sql.DB, assetID string) (*AnnouncementRow, error) {
	var (
		lastAnnounced string
		expiresAt     string
		status        string
		createdAt     string
		updatedAt     string
	)
	err := db.QueryRowContext(ctx,
		`SELECT last_announced_at, expires_at, status, created_at, updated_at
		 FROM announcements
		 WHERE asset_id = ?`,
		assetID,
	).Scan(&lastAnnounced, &expiresAt, &status, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query announcement for asset %q: %w", assetID, err)
	}

	row := &AnnouncementRow{
		AssetID: assetID,
		Status:  status,
	}
	if lastAnnounced != "" {
		t, err := time.Parse(time.RFC3339, lastAnnounced)
		if err != nil {
			return nil, fmt.Errorf("parse last_announced_at for asset %q: %w", assetID, err)
		}
		row.LastAnnouncedAt = t.UTC()
	}
	if expiresAt != "" {
		t, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return nil, fmt.Errorf("parse expires_at for asset %q: %w", assetID, err)
		}
		row.ExpiresAt = t.UTC()
	}
	if createdAt != "" {
		t, err := time.Parse(time.RFC3339, createdAt)
		if err == nil {
			row.CreatedAt = t.UTC()
		}
	}
	if updatedAt != "" {
		t, err := time.Parse(time.RFC3339, updatedAt)
		if err == nil {
			row.UpdatedAt = t.UTC()
		}
	}
	return row, nil
}

func MarkExpiredBefore(ctx context.Context, db *sql.DB, cutoffUnix int64) (int, error) {
	cutoff := time.Unix(cutoffUnix, 0).UTC().Format(time.RFC3339)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	res, err := db.ExecContext(ctx,
		`UPDATE announcements
		 SET status = 'expired', updated_at = ?
		 WHERE status != 'expired'
		   AND expires_at != ''
		   AND expires_at < ?`,
		nowStr, cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("mark expired announcements: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected expired announcements: %w", err)
	}
	return int(rows), nil
}
