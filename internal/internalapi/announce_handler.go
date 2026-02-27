package internalapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"audistro-provider/internal/catalog"
	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/repository"
)

type announceRequestBody struct {
	AssetID string `json:"asset_id"`
}

type announceSummary struct {
	Attempted int `json:"attempted"`
	OK        int `json:"ok"`
	Rejected  int `json:"rejected"`
	Failed    int `json:"failed"`
}

type CatalogErrorSink interface {
	Set(err error)
}

var announceNow = time.Now

func NewAnnounceHandler(
	db *sql.DB,
	id *identity.Identity,
	cfg config.Config,
	client *catalog.Client,
	logger *slog.Logger,
	errorSink CatalogErrorSink,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if strings.TrimSpace(cfg.PublicBaseURL) == "" {
			writeJSONError(w, http.StatusBadRequest, "public_base_url_required")
			return
		}
		if client == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "catalog_not_configured")
			return
		}

		var body announceRequestBody
		if r.Body != nil {
			defer r.Body.Close()
			dec := json.NewDecoder(r.Body)
			if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
		}

		assetIDs := make([]string, 0)
		if strings.TrimSpace(body.AssetID) != "" {
			assetIDs = append(assetIDs, strings.TrimSpace(body.AssetID))
		} else {
			ids, err := repository.ListActiveAssetIDs(r.Context(), db)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "db_active_assets_failed")
				return
			}
			assetIDs = ids
		}

		summary := announceSummary{Attempted: len(assetIDs)}
		now := announceNow().UTC()

		for _, assetID := range assetIDs {
			announceReq, err := catalog.BuildAnnouncement(id, cfg, assetID, now)
			if err != nil {
				summary.Failed++
				_ = repository.UpsertAnnouncementStatus(r.Context(), db, assetID, time.Time{}, time.Time{}, "failed", now)
				if errorSink != nil {
					errorSink.Set(err)
				}
				continue
			}

			_, err = client.Announce(r.Context(), id, announceReq)
			expiresAt := time.Unix(announceReq.ExpiresAt, 0).UTC()
			if err == nil {
				summary.OK++
				_ = repository.UpsertAnnouncementStatus(r.Context(), db, assetID, now, expiresAt, "ok", now)
				if errorSink != nil {
					errorSink.Set(nil)
				}
				continue
			}

			if errors.Is(err, catalog.ErrNotFound) {
				summary.Rejected++
				_ = repository.UpsertAnnouncementStatus(r.Context(), db, assetID, now, expiresAt, "rejected", now)
			} else {
				summary.Failed++
				_ = repository.UpsertAnnouncementStatus(r.Context(), db, assetID, now, expiresAt, "failed", now)
			}
			if errorSink != nil {
				errorSink.Set(err)
			}
			logger.Error("announce failed",
				slog.String("asset_id", assetID),
				slog.String("error", err.Error()),
			)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(summary)
	})
}
