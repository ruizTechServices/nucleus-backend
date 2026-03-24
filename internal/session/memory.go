package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidBootstrapToken = errors.New("invalid bootstrap token")
	ErrInvalidSessionToken   = errors.New("invalid session token")
	ErrSessionExpired        = errors.New("session expired")
)

type Config struct {
	BootstrapToken string
	SessionTTL     time.Duration
	Now            func() time.Time
}

type MemoryService struct {
	mu      sync.RWMutex
	now     func() time.Time
	ttl     time.Duration
	token   string
	records map[string]Metadata
}

func NewMemoryService(config Config) *MemoryService {
	now := config.Now
	if now == nil {
		now = time.Now
	}

	ttl := config.SessionTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}

	return &MemoryService{
		now:     now,
		ttl:     ttl,
		token:   config.BootstrapToken,
		records: make(map[string]Metadata),
	}
}

func (s *MemoryService) Bootstrap(ctx context.Context, request BootstrapRequest) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}

	if s.token == "" || strings.TrimSpace(request.BootstrapToken) == "" || request.BootstrapToken != s.token {
		return Metadata{}, ErrInvalidBootstrapToken
	}

	startedAt := s.now().UTC()

	sessionID, err := randomID("sess")
	if err != nil {
		return Metadata{}, err
	}

	sessionToken, err := randomID("st")
	if err != nil {
		return Metadata{}, err
	}

	metadata := Metadata{
		SessionID:     sessionID,
		SessionToken:  sessionToken,
		ClientName:    request.ClientName,
		ClientVersion: request.ClientVersion,
		TrustLevel:    TrustLevelTrustedLocalClient,
		StartedAt:     startedAt,
		ExpiresAt:     startedAt.Add(s.ttl),
	}

	s.mu.Lock()
	s.records[sessionToken] = metadata
	s.mu.Unlock()

	return metadata, nil
}

func (s *MemoryService) Validate(ctx context.Context, sessionToken string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}

	if strings.TrimSpace(sessionToken) == "" {
		return Metadata{}, ErrInvalidSessionToken
	}

	s.mu.RLock()
	metadata, ok := s.records[sessionToken]
	s.mu.RUnlock()
	if !ok {
		return Metadata{}, ErrInvalidSessionToken
	}

	if !s.now().UTC().Before(metadata.ExpiresAt) {
		s.mu.Lock()
		delete(s.records, sessionToken)
		s.mu.Unlock()
		return Metadata{}, ErrSessionExpired
	}

	return metadata, nil
}

func randomID(prefix string) (string, error) {
	var payload [12]byte
	if _, err := rand.Read(payload[:]); err != nil {
		return "", err
	}

	return prefix + "_" + hex.EncodeToString(payload[:]), nil
}
