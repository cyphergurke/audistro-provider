package internalapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"audistro-provider/internal/repository"
	"audistro-provider/internal/scanner"
)

type RescanResponse struct {
	ScannedAssets int   `json:"scanned_assets"`
	ActiveAssets  int   `json:"active_assets"`
	MissingAssets int   `json:"missing_assets"`
	DurationMS    int64 `json:"duration_ms"`
}

func RunRescan(ctx context.Context, db *sql.DB, sc *scanner.Scanner) (RescanResponse, error) {
	result, err := sc.Scan(ctx)
	if err != nil {
		return RescanResponse{}, err
	}

	now := time.Now().UTC()
	records := make([]repository.AssetRecord, 0, len(result.Assets))
	seenIDs := make([]string, 0, len(result.Assets))
	for _, asset := range result.Assets {
		records = append(records, repository.AssetRecord{
			AssetID:            asset.AssetID,
			MasterPlaylistPath: asset.MasterPlaylistPath,
			ContentHash:        asset.ContentHash,
			FileCount:          asset.FileCount,
			TotalSizeBytes:     asset.TotalSizeBytes,
			LastScannedAt:      asset.LastScannedAt,
		})
		seenIDs = append(seenIDs, asset.AssetID)
	}

	if err := repository.UpsertAssets(ctx, db, records, now); err != nil {
		return RescanResponse{}, err
	}
	missing, err := repository.MarkMissing(ctx, db, seenIDs, now)
	if err != nil {
		return RescanResponse{}, err
	}
	active, err := repository.CountActive(ctx, db)
	if err != nil {
		return RescanResponse{}, err
	}

	return RescanResponse{
		ScannedAssets: result.ScannedAssets,
		ActiveAssets:  active,
		MissingAssets: missing,
		DurationMS:    result.Duration.Milliseconds(),
	}, nil
}

func NewRescanHandler(db *sql.DB, sc *scanner.Scanner, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}

		resp, err := RunRescan(r.Context(), db, sc)
		if err != nil {
			logger.Error("rescan failed", slog.String("error", err.Error()))
			writeJSONError(w, http.StatusInternalServerError, "rescan_failed")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
