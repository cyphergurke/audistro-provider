package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type StorageMode string

const (
	StorageModeFilesystem StorageMode = "filesystem"
	StorageModeProxy      StorageMode = "proxy"
	StorageModeHybrid     StorageMode = "hybrid"
)

type Config struct {
	Env                            string
	HTTPAddr                       string
	PublicBaseURL                  string
	AllowInsecurePublicURL         bool
	DataPath                       string
	IdentityPath                   string
	DBPath                         string
	ScanOnStartup                  bool
	EnableJobs                     bool
	InternalEnable                 bool
	InternalAllowedCIDRs           string
	TrustProxyHeaders              bool
	TrustedProxyCIDRs              string
	ShutdownTimeoutSeconds         int
	AssetsSubdir                   string
	MaxSegmentBytes                int64
	RateLimitRPS                   int
	RateLimitBurst                 int
	EnableCORS                     bool
	CORSAllowedOrigins             string
	CORSAllowCredentials           bool
	EnforceNoSymlinks              bool
	TrustProxyAddresses            bool
	RescanIntervalSeconds          int
	AnnounceSweepIntervalSeconds   int
	ReannounceThresholdSeconds     int
	BackoffBaseMillis              int
	BackoffMaxSeconds              int
	RejectedRetrySeconds           int
	UnauthorizedRetrySeconds       int
	AnnouncementExpiryGraceSeconds int
	StorageMode                    StorageMode
	OriginBaseURL                  string
	OriginAuthMode                 string
	OriginHMACSecretPath           string
	OriginHMACKeyID                string
	OriginAuthIncludeQuery         bool
	ProxyMaxUpstreamConcurrency    int
	Transport                      string
	AnnouncePriority               int
	AnnounceExpiresSeconds         int
	CatalogTimeoutSeconds          int
	CatalogBaseURL                 string
	AnnounceInterval               string
	Region                         string
	PrivateKeyPath                 string
	InternalEnableExplicit         bool
	InternalAllowedCIDRsExplicit   bool
}

func Load() Config {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("AUDISTRO_ENV")))
	if env == "" {
		env = strings.ToLower(strings.TrimSpace(getenv("PROVIDER_ENV", "prod")))
	}
	if env != "dev" && env != "prod" && env != "test" {
		env = "prod"
	}
	dataPath := getenv("PROVIDER_DATA_PATH", "./data")
	identityPath := os.Getenv("PROVIDER_IDENTITY_PATH")
	if identityPath == "" {
		identityPath = filepath.Join(dataPath, "provider_identity.json")
	}
	dbPath := os.Getenv("PROVIDER_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(dataPath, "provider.db")
	}

	internalEnableDefault := false
	if env == "dev" || env == "test" {
		internalEnableDefault = true
	}
	internalEnable, internalEnableExplicit := getenvBoolWithExplicit("PROVIDER_INTERNAL_ENABLE", internalEnableDefault)
	internalAllowedCIDRs, internalAllowedCIDRsExplicit := getenvWithExplicit("PROVIDER_INTERNAL_ALLOWED_CIDRS")
	if strings.TrimSpace(internalAllowedCIDRs) == "" && env == "dev" {
		internalAllowedCIDRs = "127.0.0.1/32,::1/128"
	}

	return Config{
		Env:                            env,
		HTTPAddr:                       getenv("PROVIDER_HTTP_ADDR", ":8080"),
		PublicBaseURL:                  os.Getenv("PROVIDER_PUBLIC_BASE_URL"),
		AllowInsecurePublicURL:         getenvBool("PROVIDER_ALLOW_INSECURE_PUBLIC_URL", false),
		DataPath:                       dataPath,
		IdentityPath:                   identityPath,
		DBPath:                         dbPath,
		ScanOnStartup:                  getenvBool("PROVIDER_SCAN_ON_STARTUP", true),
		EnableJobs:                     getenvBool("PROVIDER_ENABLE_JOBS", true),
		InternalEnable:                 internalEnable,
		InternalAllowedCIDRs:           internalAllowedCIDRs,
		TrustProxyHeaders:              getenvBool("PROVIDER_TRUST_PROXY_HEADERS", false),
		TrustedProxyCIDRs:              strings.TrimSpace(os.Getenv("PROVIDER_TRUSTED_PROXY_CIDRS")),
		ShutdownTimeoutSeconds:         getenvInt("PROVIDER_SHUTDOWN_TIMEOUT_SECONDS", 15),
		AssetsSubdir:                   getenv("PROVIDER_ASSETS_SUBDIR", "assets"),
		MaxSegmentBytes:                getenvInt64("PROVIDER_MAX_SEGMENT_BYTES", 67108864),
		RateLimitRPS:                   getenvInt("PROVIDER_RATE_LIMIT_RPS", 20),
		RateLimitBurst:                 getenvInt("PROVIDER_RATE_LIMIT_BURST", 40),
		EnableCORS:                     getenvBool("PROVIDER_ENABLE_CORS", false),
		CORSAllowedOrigins:             os.Getenv("PROVIDER_CORS_ALLOWED_ORIGINS"),
		CORSAllowCredentials:           getenvBool("PROVIDER_CORS_ALLOW_CREDENTIALS", false),
		EnforceNoSymlinks:              getenvBool("PROVIDER_ENFORCE_NO_SYMLINKS", true),
		TrustProxyAddresses:            false,
		RescanIntervalSeconds:          getenvInt("PROVIDER_RESCAN_INTERVAL_SECONDS", 300),
		AnnounceSweepIntervalSeconds:   getenvInt("PROVIDER_ANNOUNCE_SWEEP_INTERVAL_SECONDS", 300),
		ReannounceThresholdSeconds:     getenvInt("PROVIDER_REANNOUNCE_THRESHOLD_SECONDS", 86400),
		BackoffBaseMillis:              getenvInt("PROVIDER_BACKOFF_BASE_MILLIS", 2000),
		BackoffMaxSeconds:              getenvInt("PROVIDER_BACKOFF_MAX_SECONDS", 600),
		RejectedRetrySeconds:           getenvInt("PROVIDER_REJECTED_RETRY_SECONDS", 86400),
		UnauthorizedRetrySeconds:       getenvInt("PROVIDER_UNAUTHORIZED_RETRY_SECONDS", 3600),
		AnnouncementExpiryGraceSeconds: getenvInt("PROVIDER_ANNOUNCEMENT_EXPIRY_GRACE_SECONDS", 86400),
		StorageMode:                    StorageMode(strings.ToLower(strings.TrimSpace(getenv("PROVIDER_STORAGE_MODE", string(StorageModeFilesystem))))),
		OriginBaseURL:                  strings.TrimSpace(os.Getenv("PROVIDER_ORIGIN_BASE_URL")),
		OriginAuthMode:                 strings.ToLower(strings.TrimSpace(getenv("PROVIDER_ORIGIN_AUTH_MODE", "none"))),
		OriginHMACSecretPath:           strings.TrimSpace(os.Getenv("PROVIDER_ORIGIN_HMAC_SECRET_PATH")),
		OriginHMACKeyID:                getenv("PROVIDER_ORIGIN_HMAC_KEY_ID", "v1"),
		OriginAuthIncludeQuery:         getenvBool("PROVIDER_ORIGIN_AUTH_INCLUDE_QUERY", true),
		ProxyMaxUpstreamConcurrency:    getenvInt("PROVIDER_PROXY_MAX_UPSTREAM_CONCURRENCY", 64),
		Transport:                      getenv("PROVIDER_TRANSPORT", "https"),
		AnnouncePriority:               getenvInt("PROVIDER_ANNOUNCE_PRIORITY", 10),
		AnnounceExpiresSeconds:         getenvInt("PROVIDER_ANNOUNCE_EXPIRES_SECONDS", 604800),
		CatalogTimeoutSeconds:          getenvInt("PROVIDER_CATALOG_TIMEOUT_SECONDS", 10),
		CatalogBaseURL:                 os.Getenv("PROVIDER_CATALOG_BASE_URL"),
		AnnounceInterval:               os.Getenv("PROVIDER_ANNOUNCE_INTERVAL"),
		Region:                         os.Getenv("PROVIDER_REGION"),
		PrivateKeyPath:                 os.Getenv("PROVIDER_PRIVATE_KEY_PATH"),
		InternalEnableExplicit:         internalEnableExplicit,
		InternalAllowedCIDRsExplicit:   internalAllowedCIDRsExplicit,
	}
}

func (c Config) Validate() error {
	mode := StorageMode(strings.ToLower(strings.TrimSpace(string(c.StorageMode))))
	switch mode {
	case "", StorageModeFilesystem:
		mode = StorageModeFilesystem
	case StorageModeProxy, StorageModeHybrid:
	default:
		return fmt.Errorf("invalid PROVIDER_STORAGE_MODE: %q", c.StorageMode)
	}

	if c.MaxSegmentBytes <= 0 {
		return fmt.Errorf("PROVIDER_MAX_SEGMENT_BYTES must be > 0")
	}
	if c.RateLimitRPS <= 0 {
		return fmt.Errorf("PROVIDER_RATE_LIMIT_RPS must be > 0")
	}
	if c.RateLimitBurst <= 0 {
		return fmt.Errorf("PROVIDER_RATE_LIMIT_BURST must be > 0")
	}
	if c.ShutdownTimeoutSeconds <= 0 {
		return fmt.Errorf("PROVIDER_SHUTDOWN_TIMEOUT_SECONDS must be > 0")
	}
	if c.ProxyMaxUpstreamConcurrency <= 0 {
		return fmt.Errorf("PROVIDER_PROXY_MAX_UPSTREAM_CONCURRENCY must be > 0")
	}

	if strings.TrimSpace(c.DataPath) == "" {
		return fmt.Errorf("PROVIDER_DATA_PATH must not be empty")
	}
	if err := os.MkdirAll(c.DataPath, 0o700); err != nil {
		return fmt.Errorf("create PROVIDER_DATA_PATH: %w", err)
	}
	assetsRoot := filepath.Join(c.DataPath, c.AssetsSubdir)
	if err := os.MkdirAll(assetsRoot, 0o700); err != nil {
		return fmt.Errorf("create assets root: %w", err)
	}

	var publicURL *url.URL
	var err error
	if strings.TrimSpace(c.PublicBaseURL) != "" {
		publicURL, err = validateURL(c.PublicBaseURL, "PROVIDER_PUBLIC_BASE_URL")
		if err != nil {
			return err
		}
		if !strings.EqualFold(publicURL.Scheme, "https") && !c.AllowInsecurePublicURL {
			return fmt.Errorf("PROVIDER_PUBLIC_BASE_URL must use https unless PROVIDER_ALLOW_INSECURE_PUBLIC_URL=true")
		}
	}

	if mode == StorageModeProxy || mode == StorageModeHybrid {
		if strings.TrimSpace(c.OriginBaseURL) == "" {
			return fmt.Errorf("PROVIDER_ORIGIN_BASE_URL is required when PROVIDER_STORAGE_MODE is %q", mode)
		}
		if _, err := validateURL(c.OriginBaseURL, "PROVIDER_ORIGIN_BASE_URL"); err != nil {
			return err
		}
	}

	switch strings.ToLower(strings.TrimSpace(c.OriginAuthMode)) {
	case "", "none":
	case "hmac":
		secretPath := strings.TrimSpace(c.OriginHMACSecretPath)
		if secretPath == "" {
			return fmt.Errorf("PROVIDER_ORIGIN_HMAC_SECRET_PATH is required when PROVIDER_ORIGIN_AUTH_MODE=hmac")
		}
		secretBytes, err := os.ReadFile(secretPath)
		if err != nil {
			return fmt.Errorf("read PROVIDER_ORIGIN_HMAC_SECRET_PATH: %w", err)
		}
		secretBytes = bytes.TrimRight(secretBytes, "\r\n")
		if len(secretBytes) < 16 {
			return fmt.Errorf("PROVIDER_ORIGIN_HMAC_SECRET_PATH secret must be at least 16 bytes")
		}
	default:
		return fmt.Errorf("invalid PROVIDER_ORIGIN_AUTH_MODE: %q", c.OriginAuthMode)
	}

	if strings.TrimSpace(c.CatalogBaseURL) != "" {
		if _, err := validateURL(c.CatalogBaseURL, "PROVIDER_CATALOG_BASE_URL"); err != nil {
			return err
		}
		if publicURL == nil {
			return fmt.Errorf("PROVIDER_PUBLIC_BASE_URL is required when PROVIDER_CATALOG_BASE_URL is set")
		}
	}

	if c.EnableCORS {
		allowed := splitCSV(c.CORSAllowedOrigins)
		if len(allowed) == 0 {
			return fmt.Errorf("PROVIDER_CORS_ALLOWED_ORIGINS must not be empty when PROVIDER_ENABLE_CORS=true")
		}
		for _, origin := range allowed {
			if origin == "*" {
				return fmt.Errorf("PROVIDER_CORS_ALLOWED_ORIGINS must not contain *")
			}
		}
	}

	if c.InternalEnable {
		if strings.TrimSpace(c.InternalAllowedCIDRs) == "" {
			return fmt.Errorf("PROVIDER_INTERNAL_ALLOWED_CIDRS is required when PROVIDER_INTERNAL_ENABLE=true")
		}
		if _, err := parseCIDRs(c.InternalAllowedCIDRs); err != nil {
			return fmt.Errorf("invalid PROVIDER_INTERNAL_ALLOWED_CIDRS: %w", err)
		}
	}

	if strings.TrimSpace(c.TrustedProxyCIDRs) != "" {
		if _, err := parseCIDRs(c.TrustedProxyCIDRs); err != nil {
			return fmt.Errorf("invalid PROVIDER_TRUSTED_PROXY_CIDRS: %w", err)
		}
	}

	return nil
}

func validateURL(raw, field string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", field, err)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid %s: missing host", field)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("invalid %s: scheme must be http or https", field)
	}
	return parsed, nil
}

func splitCSV(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func parseCIDRs(csv string) ([]*net.IPNet, error) {
	entries := splitCSV(csv)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no CIDRs provided")
	}

	out := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		_, ipnet, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", entry, err)
		}
		out = append(out, ipnet)
	}
	return out, nil
}

func getenv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getenvWithExplicit(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return value, true
}

func getenvBoolWithExplicit(key string, fallback bool) (bool, bool) {
	_, ok := os.LookupEnv(key)
	if !ok {
		return fallback, false
	}
	return getenvBool(key, fallback), true
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func getenvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return b
}
