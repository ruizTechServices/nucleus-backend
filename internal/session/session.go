package session

import (
	"context"
	"time"
)

type TrustLevel string

const (
	TrustLevelUnknown            TrustLevel = "unknown"
	TrustLevelTrustedLocalClient TrustLevel = "trusted_local_client"
)

type BootstrapRequest struct {
	ClientName     string `json:"client_name"`
	ClientVersion  string `json:"client_version"`
	BootstrapToken string `json:"bootstrap_token"`
}

type Metadata struct {
	SessionID      string     `json:"session_id"`
	SessionToken   string     `json:"session_token"`
	ClientName     string     `json:"client_name,omitempty"`
	ClientVersion  string     `json:"client_version,omitempty"`
	TrustLevel     TrustLevel `json:"trust_level"`
	StartedAt      time.Time  `json:"started_at"`
	ExpiresAt      time.Time  `json:"expires_at"`
	ApprovedScopes []string   `json:"approved_scopes,omitempty"`
	Capabilities   []string   `json:"capabilities,omitempty"`
}

type Service interface {
	Bootstrap(ctx context.Context, request BootstrapRequest) (Metadata, error)
	Validate(ctx context.Context, sessionToken string) (Metadata, error)
}
