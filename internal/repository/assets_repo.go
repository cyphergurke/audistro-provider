package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type AssetRecord struct {
	AssetID            string
	MasterPlaylistPath string
	ContentHash        string
	FileCount          int
	TotalSizeBytes     int64
	LastScannedAt      time.Time
}

func UpsertAssets(ctx context.Context, db *sql.DB, assets []AssetRecord, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt := `INSERT INTO assets (
		asset_id, status, master_playlist_path, content_hash, file_count,
		total_size_bytes, last_scanned_at, created_at, updated_at, missing_at
	) VALUES (?, 'active', ?, ?, ?, ?, ?, ?, ?, NULL)
	ON CONFLICT(asset_id) DO UPDATE SET
		status='active',
		master_playlist_path=excluded.master_playlist_path,
		content_hash=excluded.content_hash,
		file_count=excluded.file_count,
		total_size_bytes=excluded.total_size_bytes,
		last_scanned_at=excluded.last_scanned_at,
		updated_at=excluded.updated_at,
		missing_at=NULL`

	nowStr := now.UTC().Format(time.RFC3339)
	for _, asset := range assets {
		lastScanned := asset.LastScannedAt.UTC().Format(time.RFC3339)
		if _, err := tx.ExecContext(ctx, stmt,
			asset.AssetID,
			asset.MasterPlaylistPath,
			asset.ContentHash,
			asset.FileCount,
			asset.TotalSizeBytes,
			lastScanned,
			nowStr,
			nowStr,
		); err != nil {
			return fmt.Errorf("upsert asset %q: %w", asset.AssetID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert tx: %w", err)
	}
	return nil
}

func MarkMissing(ctx context.Context, db *sql.DB, seenAssetIDs []string, now time.Time) (int, error) {
	nowStr := now.UTC().Format(time.RFC3339)

	if len(seenAssetIDs) == 0 {
		res, err := db.ExecContext(ctx,
			`UPDATE assets
			 SET status='missing', missing_at=?, updated_at=?
			 WHERE status != 'missing'`,
			nowStr, nowStr,
		)
		if err != nil {
			return 0, fmt.Errorf("mark missing (empty seen set): %w", err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("rows affected missing (empty seen set): %w", err)
		}
		return int(rows), nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(seenAssetIDs)), ",")
	args := make([]any, 0, len(seenAssetIDs)+2)
	args = append(args, nowStr, nowStr)
	for _, id := range seenAssetIDs {
		args = append(args, id)
	}

	query := fmt.Sprintf(`UPDATE assets
		SET status='missing', missing_at=?, updated_at=?
		WHERE status != 'missing' AND asset_id NOT IN (%s)`, placeholders)
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("mark missing: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected missing: %w", err)
	}
	return int(rows), nil
}

func CountActive(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets WHERE status = 'active'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active assets: %w", err)
	}
	return count, nil
}

func CountMissing(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets WHERE status = 'missing'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count missing assets: %w", err)
	}
	return count, nil
}

func ListActiveAssetIDs(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT asset_id FROM assets WHERE status = 'active' ORDER BY asset_id`)
	if err != nil {
		return nil, fmt.Errorf("query active assets: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			return nil, fmt.Errorf("scan active asset id: %w", err)
		}
		out = append(out, assetID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active assets: %w", err)
	}
	return out, nil
}
