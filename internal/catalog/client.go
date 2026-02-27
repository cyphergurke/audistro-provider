package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"audistro-provider/internal/config"
	"audistro-provider/internal/identity"
	"audistro-provider/internal/metrics"
)

var (
	ErrNotFound     = errors.New("catalog not found")
	ErrUnauthorized = errors.New("catalog unauthorized")
	ErrConflict     = errors.New("catalog conflict")
	ErrBadRequest   = errors.New("catalog bad request")
	ErrServer       = errors.New("catalog server error")
	ErrUnexpected   = errors.New("catalog unexpected response")
	ErrUnconfigured = errors.New("catalog client unconfigured")
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	logger     *slog.Logger
	metrics    *metrics.Metrics
}

func NewClient(rawBaseURL string, timeout time.Duration, logger *slog.Logger, metricsCollector *metrics.Metrics) (*Client, error) {
	trimmed := strings.TrimSpace(rawBaseURL)
	if trimmed == "" {
		return nil, ErrUnconfigured
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse catalog base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid catalog base url")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Client{
		baseURL:    u,
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
		metrics:    metricsCollector,
	}, nil
}

func (c *Client) EnsureProvider(ctx context.Context, id *identity.Identity, cfg config.Config) error {
	start := time.Now()
	const endpointLabel = "providers_register"
	if strings.TrimSpace(cfg.PublicBaseURL) == "" {
		c.observeCatalog(endpointLabel, "error", 0, start)
		return fmt.Errorf("public base url is required")
	}

	reqBody := EnsureProviderRequest{
		ProviderID: id.ProviderID.String(),
		PublicKey:  id.PublicKeyHexCompressed(),
		Transport:  cfg.Transport,
		BaseURL:    cfg.PublicBaseURL,
		Region:     cfg.Region,
	}

	endpoint := c.joinPath("/v1/providers")
	resp, err := c.doJSON(ctx, http.MethodPost, endpoint, reqBody)
	if err != nil {
		c.observeCatalog(endpointLabel, "error", 0, start)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.observeCatalog(endpointLabel, catalogResult(resp.StatusCode), resp.StatusCode, start)
		return statusError(resp.StatusCode)
	}
	c.observeCatalog(endpointLabel, "ok", resp.StatusCode, start)

	c.logger.Info("catalog provider ensured",
		slog.String("provider_id", id.ProviderID.String()),
		slog.String("public_key_fingerprint", id.Fingerprint()),
	)
	return nil
}

func (c *Client) Announce(ctx context.Context, id *identity.Identity, req AnnounceRequest) (*AnnounceResponse, error) {
	start := time.Now()
	const endpointLabel = "providers_announce"
	endpoint := c.joinPath("/v1/providers", id.ProviderID.String(), "announce")
	resp, err := c.doJSON(ctx, http.MethodPost, endpoint, req)
	if err != nil {
		c.observeCatalog(endpointLabel, "error", 0, start)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.observeCatalog(endpointLabel, catalogResult(resp.StatusCode), resp.StatusCode, start)
		return nil, statusError(resp.StatusCode)
	}

	var out AnnounceResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		c.observeCatalog(endpointLabel, "error", resp.StatusCode, start)
		return nil, fmt.Errorf("decode announce response: %w", err)
	}
	c.observeCatalog(endpointLabel, "ok", resp.StatusCode, start)
	return &out, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, payload any) (*http.Response, error) {
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request catalog: %w", err)
	}
	return resp, nil
}

func (c *Client) joinPath(parts ...string) string {
	u := *c.baseURL
	joined := ""
	for _, p := range parts {
		joined = path.Join(joined, p)
	}
	u.Path = path.Join(c.baseURL.Path, joined)
	return u.String()
}

func statusError(status int) error {
	switch status {
	case http.StatusBadRequest:
		return ErrBadRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	default:
		if status >= 500 {
			return ErrServer
		}
		return fmt.Errorf("%w: status=%d", ErrUnexpected, status)
	}
}

func (c *Client) observeCatalog(endpoint, result string, status int, start time.Time) {
	if c.metrics == nil {
		return
	}
	c.metrics.ObserveCatalog(endpoint, result, status, time.Since(start))
}

func catalogResult(status int) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "unauthorized"
	case status == http.StatusNotFound:
		return "rejected"
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "ok"
	}
}
