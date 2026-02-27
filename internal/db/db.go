package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/glebarez/go-sqlite"

	"audistro-provider/internal/config"
)

const currentSchemaVersion = 1

func Open(cfg config.Config) (*sql.DB, error) {
	if err := os.MkdirAll(cfg.DataPath, 0o700); err != nil {
		return nil, fmt.Errorf("create data path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", pragma, err)
		}
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}

func Migrate(db *sql.DB) error {
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_version (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			version INTEGER NOT NULL
		);`,
		`INSERT INTO schema_version (id, version)
		 VALUES (1, 1)
		 ON CONFLICT(id) DO NOTHING;`,
		`CREATE TABLE IF NOT EXISTS assets (
			asset_id TEXT PRIMARY KEY,
			status TEXT NOT NULL CHECK (status IN ('active','missing')),
			master_playlist_path TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			file_count INTEGER NOT NULL,
			total_size_bytes INTEGER NOT NULL,
			last_scanned_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			missing_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS announcements (
			asset_id TEXT PRIMARY KEY,
			last_announced_at TEXT,
			expires_at TEXT,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(asset_id) REFERENCES assets(asset_id)
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_announcements_asset_id ON announcements(asset_id);`,
	}

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration statement failed: %w", err)
		}
	}

	var version int
	if err := tx.QueryRowContext(ctx, `SELECT version FROM schema_version WHERE id = 1`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version != currentSchemaVersion {
		return fmt.Errorf("unsupported schema version %d", version)
	}

	if err := ensureAnnouncementColumn(ctx, tx, "last_announced_at", "TEXT"); err != nil {
		return err
	}
	if err := ensureAnnouncementColumn(ctx, tx, "expires_at", "TEXT"); err != nil {
		return err
	}
	if err := ensureAnnouncementColumn(ctx, tx, "status", "TEXT NOT NULL DEFAULT 'failed'"); err != nil {
		return err
	}
	if err := ensureAnnouncementColumn(ctx, tx, "created_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureAnnouncementColumn(ctx, tx, "updated_at", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}
	return nil
}

func ensureAnnouncementColumn(ctx context.Context, tx *sql.Tx, column, columnDef string) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(announcements)`)
	if err != nil {
		return fmt.Errorf("read announcements table info: %w", err)
	}
	defer rows.Close()

	exists := false
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan announcements table info: %w", err)
		}
		if name == column {
			exists = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate announcements table info: %w", err)
	}
	if exists {
		return nil
	}

	stmt := fmt.Sprintf("ALTER TABLE announcements ADD COLUMN %s %s", column, columnDef)
	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("add announcements column %q: %w", column, err)
	}
	return nil
}
