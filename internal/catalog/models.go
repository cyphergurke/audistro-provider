package catalog

type EnsureProviderRequest struct {
	ProviderID string `json:"provider_id"`
	PublicKey  string `json:"public_key"`
	Transport  string `json:"transport"`
	BaseURL    string `json:"base_url"`
	Region     string `json:"region,omitempty"`
}

type AnnounceRequest struct {
	AssetID          string `json:"asset_id"`
	Transport        string `json:"transport"`
	BaseURL          string `json:"base_url"`
	Priority         int    `json:"priority"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
	ExpiresAt        int64  `json:"expires_at"`
	Nonce            string `json:"nonce"`
	Signature        string `json:"signature"`
}

type AnnounceResponse struct {
	Status string `json:"status,omitempty"`
}
