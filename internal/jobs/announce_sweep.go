package jobs

import (
	"context"
	"database/sql"
	"time"

	"audistro-provider/internal/catalog"
	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/metrics"
	"audistro-provider/internal/repository"
)

type CatalogAnnouncer interface {
	Announce(ctx context.Context, id *identity.Identity, req catalog.AnnounceRequest) error
}

type AnnounceSweepResult struct {
	Attempted int
	OK        int
	Rejected  int
	Failed    int
	Deferred  int
}

func RunAnnounceSweep(
	ctx context.Context,
	db *sql.DB,
	announcer CatalogAnnouncer,
	id *identity.Identity,
	cfg config.Config,
	backoff *BackoffState,
	clock Clock,
	metricsCollector *metrics.Metrics,
) (AnnounceSweepResult, error) {
	if announcer == nil {
		return AnnounceSweepResult{}, nil
	}

	now := clock.Now().UTC()
	activeIDs, err := repository.ListActiveAssetIDs(ctx, db)
	if err != nil {
		return AnnounceSweepResult{}, err
	}

	result := AnnounceSweepResult{}
	threshold := now.Add(time.Duration(cfg.ReannounceThresholdSeconds) * time.Second)
	for _, assetID := range activeIDs {
		row, err := repository.GetAnnouncement(ctx, db, assetID)
		if err != nil {
			result.Failed++
			continue
		}

		if !needsReannounce(row, threshold) {
			continue
		}

		if !backoff.CanAttempt(assetID, now) {
			result.Deferred++
			continue
		}

		result.Attempted++
		req, err := catalog.BuildAnnouncement(id, cfg, assetID, now)
		if err != nil {
			result.Failed++
			if metricsCollector != nil {
				metricsCollector.ObserveAnnounce("failed")
			}
			backoff.Record(assetID, ClassPermanent, now)
			_ = repository.UpsertAnnouncementStatus(ctx, db, assetID, time.Time{}, time.Time{}, "failed", now)
			continue
		}

		err = announcer.Announce(ctx, id, req)
		class := ClassifyError(err)
		expiresAt := time.Unix(req.ExpiresAt, 0).UTC()

		switch class {
		case ClassOK:
			result.OK++
			if metricsCollector != nil {
				metricsCollector.ObserveAnnounce("ok")
			}
			_ = repository.UpsertAnnouncementStatus(ctx, db, assetID, now, expiresAt, "ok", now)
		case ClassRejected:
			result.Rejected++
			if metricsCollector != nil {
				metricsCollector.ObserveAnnounce("rejected")
			}
			_ = repository.UpsertAnnouncementStatus(ctx, db, assetID, now, expiresAt, "rejected", now)
		case ClassUnauthorized:
			result.Failed++
			if metricsCollector != nil {
				metricsCollector.ObserveAnnounce("unauthorized")
			}
			_ = repository.UpsertAnnouncementStatus(ctx, db, assetID, now, expiresAt, "failed", now)
		default:
			result.Failed++
			if metricsCollector != nil {
				metricsCollector.ObserveAnnounce("failed")
			}
			_ = repository.UpsertAnnouncementStatus(ctx, db, assetID, now, expiresAt, "failed", now)
		}
		backoff.Record(assetID, class, now)
	}

	return result, nil
}

func needsReannounce(row *repository.AnnouncementRow, threshold time.Time) bool {
	if row == nil {
		return true
	}
	if row.Status != "ok" {
		return true
	}
	if row.ExpiresAt.IsZero() {
		return true
	}
	return row.ExpiresAt.Before(threshold)
}

type catalogAnnouncerAdapter struct {
	client *catalog.Client
}

func NewCatalogAnnouncer(client *catalog.Client) CatalogAnnouncer {
	if client == nil {
		return nil
	}
	return &catalogAnnouncerAdapter{client: client}
}

func (a *catalogAnnouncerAdapter) Announce(ctx context.Context, id *identity.Identity, req catalog.AnnounceRequest) error {
	_, err := a.client.Announce(ctx, id, req)
	return err
}
