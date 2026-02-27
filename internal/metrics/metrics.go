package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

type Metrics struct {
	Registry *prometheus.Registry

	HTTPRequestsTotal      *prometheus.CounterVec
	HTTPRequestDuration    *prometheus.HistogramVec
	HTTPResponseBytes      *prometheus.HistogramVec
	HTTPInFlight           *prometheus.GaugeVec
	ScannerScansTotal      *prometheus.CounterVec
	ScannerScanDuration    *prometheus.HistogramVec
	CatalogRequestsTotal   *prometheus.CounterVec
	CatalogRequestDuration *prometheus.HistogramVec
	AnnouncementsTotal     *prometheus.CounterVec
	AnnounceSweepsTotal    *prometheus.CounterVec
	ActiveAssets           prometheus.Gauge
	MissingAssets          prometheus.Gauge
	DeferredAnnounces      prometheus.Gauge
	ProxyRequestsTotal     *prometheus.CounterVec
	ProxyRequestDuration   *prometheus.HistogramVec
	ProxyUpstreamInFlight  prometheus.Gauge
	ProxyUpstreamBusyTotal prometheus.Counter
}

func New(reg *prometheus.Registry) *Metrics {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}

	m := &Metrics{
		Registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audistro_provider_http_requests_total",
				Help: "Total HTTP requests handled by route, method, and status.",
			},
			[]string{"route", "method", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "audistro_provider_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds by route and method.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"route", "method"},
		),
		HTTPResponseBytes: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "audistro_provider_http_response_bytes",
				Help:    "HTTP response bytes by route and method.",
				Buckets: []float64{128, 512, 1024, 4 * 1024, 16 * 1024, 64 * 1024, 256 * 1024, 1024 * 1024, 4 * 1024 * 1024},
			},
			[]string{"route", "method"},
		),
		HTTPInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "audistro_provider_http_inflight_requests",
				Help: "Number of in-flight HTTP requests by route.",
			},
			[]string{"route"},
		),
		ScannerScansTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audistro_provider_scanner_scans_total",
				Help: "Total number of filesystem scans by result.",
			},
			[]string{"result"},
		),
		ScannerScanDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "audistro_provider_scanner_scan_duration_seconds",
				Help:    "Filesystem scan duration in seconds by result.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		),
		CatalogRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audistro_provider_catalog_requests_total",
				Help: "Total catalog HTTP requests by endpoint, result, and status.",
			},
			[]string{"endpoint", "result", "status"},
		),
		CatalogRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "audistro_provider_catalog_request_duration_seconds",
				Help:    "Catalog HTTP request duration in seconds by endpoint and result.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"endpoint", "result"},
		),
		AnnouncementsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audistro_provider_announcements_total",
				Help: "Total announcement attempts by result.",
			},
			[]string{"result"},
		),
		AnnounceSweepsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audistro_provider_announce_sweeps_total",
				Help: "Total announce sweep runs by result.",
			},
			[]string{"result"},
		),
		ActiveAssets: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "audistro_provider_active_assets",
				Help: "Current number of active assets.",
			},
		),
		MissingAssets: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "audistro_provider_missing_assets",
				Help: "Current number of missing assets.",
			},
		),
		DeferredAnnounces: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "audistro_provider_deferred_announces",
				Help: "Current number of deferred announces due to backoff.",
			},
		),
		ProxyRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audistro_provider_proxy_requests_total",
				Help: "Total proxied asset requests by result and status.",
			},
			[]string{"result", "status"},
		),
		ProxyRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "audistro_provider_proxy_request_duration_seconds",
				Help:    "Proxied request duration in seconds by result.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"result"},
		),
		ProxyUpstreamInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "audistro_provider_proxy_upstream_inflight",
				Help: "Current in-flight upstream proxy requests.",
			},
		),
		ProxyUpstreamBusyTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "audistro_provider_proxy_upstream_busy_total",
				Help: "Total proxied requests rejected due to upstream concurrency saturation.",
			},
		),
	}

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPResponseBytes,
		m.HTTPInFlight,
		m.ScannerScansTotal,
		m.ScannerScanDuration,
		m.CatalogRequestsTotal,
		m.CatalogRequestDuration,
		m.AnnouncementsTotal,
		m.AnnounceSweepsTotal,
		m.ActiveAssets,
		m.MissingAssets,
		m.DeferredAnnounces,
		m.ProxyRequestsTotal,
		m.ProxyRequestDuration,
		m.ProxyUpstreamInFlight,
		m.ProxyUpstreamBusyTotal,
	)

	return m
}

func (m *Metrics) ObserveHTTPRequest(route, method string, status int, duration time.Duration, bytesOut int64) {
	if m == nil {
		return
	}
	statusLabel := strconv.Itoa(status)
	m.HTTPRequestsTotal.WithLabelValues(route, method, statusLabel).Inc()
	m.HTTPRequestDuration.WithLabelValues(route, method).Observe(duration.Seconds())
	if bytesOut >= 0 {
		m.HTTPResponseBytes.WithLabelValues(route, method).Observe(float64(bytesOut))
	}
}

func (m *Metrics) IncInFlight(route string) func() {
	if m == nil {
		return func() {}
	}
	g := m.HTTPInFlight.WithLabelValues(route)
	g.Inc()
	return func() { g.Dec() }
}

func (m *Metrics) ObserveScan(result string, duration time.Duration) {
	if m == nil {
		return
	}
	m.ScannerScansTotal.WithLabelValues(result).Inc()
	m.ScannerScanDuration.WithLabelValues(result).Observe(duration.Seconds())
}

func (m *Metrics) ObserveCatalog(endpoint, result string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	m.CatalogRequestsTotal.WithLabelValues(endpoint, result, strconv.Itoa(status)).Inc()
	m.CatalogRequestDuration.WithLabelValues(endpoint, result).Observe(duration.Seconds())
}

func (m *Metrics) ObserveAnnounce(result string) {
	if m == nil {
		return
	}
	m.AnnouncementsTotal.WithLabelValues(result).Inc()
}

func (m *Metrics) ObserveAnnounceSweep(result string) {
	if m == nil {
		return
	}
	m.AnnounceSweepsTotal.WithLabelValues(result).Inc()
}

func (m *Metrics) SetActiveAssets(n int) {
	if m == nil {
		return
	}
	m.ActiveAssets.Set(float64(n))
}

func (m *Metrics) SetMissingAssets(n int) {
	if m == nil {
		return
	}
	m.MissingAssets.Set(float64(n))
}

func (m *Metrics) SetDeferredAnnounces(n int) {
	if m == nil {
		return
	}
	m.DeferredAnnounces.Set(float64(n))
}

func (m *Metrics) ObserveProxy(status int, duration time.Duration, result string) {
	if m == nil {
		return
	}
	m.ProxyRequestsTotal.WithLabelValues(result, strconv.Itoa(status)).Inc()
	m.ProxyRequestDuration.WithLabelValues(result).Observe(duration.Seconds())
}

func (m *Metrics) IncProxyUpstreamInFlight() func() {
	if m == nil {
		return func() {}
	}
	m.ProxyUpstreamInFlight.Inc()
	return func() {
		m.ProxyUpstreamInFlight.Dec()
	}
}

func (m *Metrics) IncProxyUpstreamBusy() {
	if m == nil {
		return
	}
	m.ProxyUpstreamBusyTotal.Inc()
}
