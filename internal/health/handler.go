package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type ActiveAssetsCounter func(context.Context) (int, error)
type CatalogStatusProvider func() (bool, string)
type JobsStatusSnapshot struct {
	JobsEnabled            bool
	LastRescanAt           time.Time
	LastAnnounceSweepAt    time.Time
	DeferredAnnouncesCount int
}
type JobsStatusProvider func() JobsStatusSnapshot

type Response struct {
	Status                 string    `json:"status"`
	Service                string    `json:"service"`
	Version                string    `json:"version"`
	Time                   time.Time `json:"time"`
	Region                 string    `json:"region,omitempty"`
	PublicBaseURL          string    `json:"public_base_url,omitempty"`
	ProviderID             string    `json:"provider_id"`
	PublicKey              string    `json:"public_key"`
	ActiveAssetsCount      *int      `json:"active_assets_count,omitempty"`
	Warning                string    `json:"warning,omitempty"`
	CatalogEnabled         bool      `json:"catalog_enabled"`
	CatalogLastError       string    `json:"catalog_last_error,omitempty"`
	MetricsEnabled         bool      `json:"metrics_enabled"`
	JobsEnabled            bool      `json:"jobs_enabled"`
	LastRescanAt           *int64    `json:"last_rescan_at,omitempty"`
	LastAnnounceSweepAt    *int64    `json:"last_announce_sweep_at,omitempty"`
	DeferredAnnouncesCount int       `json:"deferred_announces_count"`
}

func Handler(
	service, version, region, publicBaseURL, providerID, publicKey string,
	counter ActiveAssetsCounter,
	catalogStatus CatalogStatusProvider,
	jobsStatus JobsStatusProvider,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Status:         "ok",
			Service:        service,
			Version:        version,
			Time:           time.Now().UTC(),
			Region:         region,
			PublicBaseURL:  publicBaseURL,
			ProviderID:     providerID,
			PublicKey:      publicKey,
			CatalogEnabled: false,
			MetricsEnabled: true,
			JobsEnabled:    false,
		}
		if counter != nil {
			active, err := counter(r.Context())
			if err != nil {
				resp.Warning = "active_assets_count_unavailable"
			} else {
				resp.ActiveAssetsCount = &active
			}
		}
		if catalogStatus != nil {
			enabled, lastError := catalogStatus()
			resp.CatalogEnabled = enabled
			resp.CatalogLastError = lastError
		}
		if jobsStatus != nil {
			snapshot := jobsStatus()
			resp.JobsEnabled = snapshot.JobsEnabled
			resp.DeferredAnnouncesCount = snapshot.DeferredAnnouncesCount
			if !snapshot.LastRescanAt.IsZero() {
				t := snapshot.LastRescanAt.UTC().Unix()
				resp.LastRescanAt = &t
			}
			if !snapshot.LastAnnounceSweepAt.IsZero() {
				t := snapshot.LastAnnounceSweepAt.UTC().Unix()
				resp.LastAnnounceSweepAt = &t
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
