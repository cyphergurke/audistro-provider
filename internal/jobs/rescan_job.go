package jobs

import (
	"context"
	"database/sql"

	"audistro-provider/internal/repository"
	"audistro-provider/internal/scanner"
)

type RescanResult struct {
	ScannedAssets int
	ActiveAssets  int
	MissingAssets int
}

func RunRescanJob(ctx context.Context, db *sql.DB, sc *scanner.Scanner, clock Clock) (RescanResult, error) {
	scanResult, err := sc.Scan(ctx)
	if err != nil {
		return RescanResult{}, err
	}

	now := clock.Now().UTC()
	records := make([]repository.AssetRecord, 0, len(scanResult.Assets))
	seen := make([]string, 0, len(scanResult.Assets))
	for _, asset := range scanResult.Assets {
		records = append(records, repository.AssetRecord{
			AssetID:            asset.AssetID,
			MasterPlaylistPath: asset.MasterPlaylistPath,
			ContentHash:        asset.ContentHash,
			FileCount:          asset.FileCount,
			TotalSizeBytes:     asset.TotalSizeBytes,
			LastScannedAt:      asset.LastScannedAt,
		})
		seen = append(seen, asset.AssetID)
	}

	if err := repository.UpsertAssets(ctx, db, records, now); err != nil {
		return RescanResult{}, err
	}
	missing, err := repository.MarkMissing(ctx, db, seen, now)
	if err != nil {
		return RescanResult{}, err
	}
	active, err := repository.CountActive(ctx, db)
	if err != nil {
		return RescanResult{}, err
	}

	return RescanResult{
		ScannedAssets: scanResult.ScannedAssets,
		ActiveAssets:  active,
		MissingAssets: missing,
	}, nil
}
