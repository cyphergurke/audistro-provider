package jobs

import (
	"context"
	"database/sql"
	"time"

	"audistro-provider/internal/repository"
)

func RunCleanupJob(ctx context.Context, db *sql.DB, graceSeconds int, clock Clock) (int, error) {
	cutoff := clock.Now().UTC().Add(-time.Duration(graceSeconds) * time.Second).Unix()
	return repository.MarkExpiredBefore(ctx, db, cutoff)
}
