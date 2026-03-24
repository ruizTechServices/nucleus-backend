package session

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryServiceBootstrapAndValidate(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	service := NewMemoryService(Config{
		BootstrapToken: "bootstrap-secret",
		SessionTTL:     15 * time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	metadata, err := service.Bootstrap(context.Background(), BootstrapRequest{
		ClientName:     "nucleus-electron",
		ClientVersion:  "0.1.0",
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected bootstrap to succeed: %v", err)
	}

	if metadata.SessionID == "" || metadata.SessionToken == "" {
		t.Fatalf("expected session identifiers to be populated: %+v", metadata)
	}

	if metadata.TrustLevel != TrustLevelTrustedLocalClient {
		t.Fatalf("expected trusted local client trust level, got %q", metadata.TrustLevel)
	}

	validated, err := service.Validate(context.Background(), metadata.SessionToken)
	if err != nil {
		t.Fatalf("expected session validation to succeed: %v", err)
	}

	if validated.SessionID != metadata.SessionID {
		t.Fatalf("expected session id %q, got %q", metadata.SessionID, validated.SessionID)
	}
}

func TestMemoryServiceRejectsInvalidBootstrapToken(t *testing.T) {
	service := NewMemoryService(Config{
		BootstrapToken: "bootstrap-secret",
	})

	_, err := service.Bootstrap(context.Background(), BootstrapRequest{
		BootstrapToken: "wrong",
	})
	if !errors.Is(err, ErrInvalidBootstrapToken) {
		t.Fatalf("expected invalid bootstrap token error, got %v", err)
	}
}

func TestMemoryServiceExpiresSessions(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	service := NewMemoryService(Config{
		BootstrapToken: "bootstrap-secret",
		SessionTTL:     time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	metadata, err := service.Bootstrap(context.Background(), BootstrapRequest{
		BootstrapToken: "bootstrap-secret",
	})
	if err != nil {
		t.Fatalf("expected bootstrap to succeed: %v", err)
	}

	now = now.Add(time.Minute)

	_, err = service.Validate(context.Background(), metadata.SessionToken)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected session expired error, got %v", err)
	}
}
