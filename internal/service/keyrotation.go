package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/your-org/sso/internal/crypto"
	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

type KeyRotationService struct {
	keyStore         store.KeyStore
	jwtSvc           *crypto.JWTService
	auditSvc         *AuditService
	transitionPeriod time.Duration
}

func NewKeyRotationService(
	keyStore store.KeyStore,
	jwtSvc *crypto.JWTService,
	auditSvc *AuditService,
	transitionPeriod time.Duration,
) *KeyRotationService {
	return &KeyRotationService{
		keyStore:         keyStore,
		jwtSvc:           jwtSvc,
		auditSvc:         auditSvc,
		transitionPeriod: transitionPeriod,
	}
}

func (s *KeyRotationService) RotateKey(ctx context.Context) (*model.KeyVersion, error) {
	activeKey, err := s.keyStore.GetActiveKey(ctx)
	if err != nil && !apperrors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("failed to get active key: %w", err)
	}

	privateKey, err := crypto.GenerateRSAKeyPair(2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new key pair: %w", err)
	}

	newKeyVersion, err := crypto.CreateKeyVersion(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create key version: %w", err)
	}

	if err := s.keyStore.StoreKey(ctx, newKeyVersion); err != nil {
		return nil, fmt.Errorf("failed to store new key: %w", err)
	}

	pubKey, err := crypto.ParsePublicKey(newKeyVersion.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	privKey, err := crypto.ParsePrivateKey(newKeyVersion.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	if err := s.jwtSvc.SetActiveKey(newKeyVersion.ID, privKey, pubKey); err != nil {
		return nil, fmt.Errorf("failed to set active key: %w", err)
	}

	if activeKey != nil {
		expiresAt := time.Now().Add(s.transitionPeriod)
		if err := s.keyStore.DeprecateKey(ctx, activeKey.ID, expiresAt); err != nil {
			slog.Warn("failed to deprecate old key", "error", err, "key_id", activeKey.ID)
		} else {
			slog.Info("old key deprecated",
				"key_id", activeKey.ID,
				"expires_at", expiresAt,
			)
		}
	}

	if s.auditSvc != nil {
		s.auditSvc.LogKeyRotated(ctx, newKeyVersion.ID)
	}

	slog.Info("key rotation completed",
		"new_key_id", newKeyVersion.ID,
		"transition_period", s.transitionPeriod,
	)

	return newKeyVersion, nil
}

func (s *KeyRotationService) CleanupExpiredKeys(ctx context.Context) (int, error) {
	keys, err := s.keyStore.ListAllKeys(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list keys: %w", err)
	}

	cleanedCount := 0
	now := time.Now()

	for _, key := range keys {
		if key.Status == model.KeyStatusDeprecated && key.ExpiresAt != nil && key.ExpiresAt.Before(now) {
			if err := s.keyStore.RevokeKey(ctx, key.ID); err != nil {
				slog.Warn("failed to revoke expired key", "error", err, "key_id", key.ID)
				continue
			}
			s.jwtSvc.RemoveKey(key.ID)
			cleanedCount++
			slog.Info("revoked expired key", "key_id", key.ID)

			if s.auditSvc != nil {
				s.auditSvc.LogKeyRevoked(ctx, key.ID)
			}
		}
	}

	return cleanedCount, nil
}

func (s *KeyRotationService) RevokeKey(ctx context.Context, keyID string) error {
	key, err := s.keyStore.GetKeyByID(ctx, keyID)
	if err != nil {
		return fmt.Errorf("failed to get key: %w", err)
	}

	if key.Status == model.KeyStatusActive {
		return fmt.Errorf("cannot revoke active key, rotate first")
	}

	if err := s.keyStore.RevokeKey(ctx, keyID); err != nil {
		return fmt.Errorf("failed to revoke key: %w", err)
	}

	s.jwtSvc.RemoveKey(keyID)

	if s.auditSvc != nil {
		s.auditSvc.LogKeyRevoked(ctx, keyID)
	}

	slog.Info("key revoked", "key_id", keyID)

	return nil
}

func (s *KeyRotationService) GetKeyStatus(ctx context.Context) ([]*model.KeyVersion, error) {
	return s.keyStore.ListAllKeys(ctx)
}

func (s *KeyRotationService) InitializeFirstKey(ctx context.Context) (*model.KeyVersion, error) {
	activeKey, err := s.keyStore.GetActiveKey(ctx)
	if err != nil && !apperrors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("failed to check for active key: %w", err)
	}

	if activeKey != nil {
		return activeKey, nil
	}

	return s.RotateKey(ctx)
}
