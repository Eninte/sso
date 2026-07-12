package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/serviceutil"
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
		return nil, serviceutil.WrapServiceError("获取活跃密钥", err)
	}

	privateKey, err := crypto.GenerateRSAKeyPair(2048)
	if err != nil {
		return nil, serviceutil.WrapServiceError("生成密钥对", err)
	}

	newKeyVersion, err := crypto.CreateKeyVersion(privateKey)
	if err != nil {
		return nil, serviceutil.WrapServiceError("创建密钥版本", err)
	}

	if err := s.keyStore.StoreKey(ctx, newKeyVersion); err != nil {
		return nil, serviceutil.WrapServiceError("存储新密钥", err)
	}

	pubKey, err := crypto.ParsePublicKey(newKeyVersion.PublicKey)
	if err != nil {
		return nil, serviceutil.WrapServiceError("解析公钥", err)
	}

	privKey, err := crypto.ParsePrivateKey(newKeyVersion.PrivateKey)
	if err != nil {
		return nil, serviceutil.WrapServiceError("解析私钥", err)
	}

	if err := s.jwtSvc.SetActiveKey(newKeyVersion.ID, privKey, pubKey); err != nil {
		return nil, serviceutil.WrapServiceError("设置活跃密钥", err)
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

	// 使用统一的审计日志工具记录密钥轮换事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventKeyRotated), "", map[string]interface{}{
		"key_id": newKeyVersion.ID,
	})

	slog.Info("key rotation completed",
		"new_key_id", newKeyVersion.ID,
		"transition_period", s.transitionPeriod,
	)

	return newKeyVersion, nil
}

func (s *KeyRotationService) CleanupExpiredKeys(ctx context.Context) (int, error) {
	keys, err := s.keyStore.ListAllKeys(ctx)
	if err != nil {
		return 0, serviceutil.WrapServiceError("列出密钥", err)
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

			// 使用统一的审计日志工具记录密钥撤销事件
			auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventKeyRevoked), "", map[string]interface{}{
				"key_id": key.ID,
			})
		}
	}

	return cleanedCount, nil
}

func (s *KeyRotationService) RevokeKey(ctx context.Context, keyID string) error {
	key, err := s.keyStore.GetKeyByID(ctx, keyID)
	if err != nil {
		return serviceutil.WrapServiceError("获取密钥", err)
	}

	if key.Status == model.KeyStatusActive {
		return fmt.Errorf("cannot revoke active key, rotate first")
	}

	if err := s.keyStore.RevokeKey(ctx, keyID); err != nil {
		return serviceutil.WrapServiceError("撤销密钥", err)
	}

	s.jwtSvc.RemoveKey(keyID)

	// 使用统一的审计日志工具记录密钥撤销事件
	auditutil.SafeAuditLog(ctx, s.auditSvc, string(model.EventKeyRevoked), "", map[string]interface{}{
		"key_id": keyID,
	})

	slog.Info("key revoked", "key_id", keyID)

	return nil
}

func (s *KeyRotationService) GetKeyStatus(ctx context.Context) ([]*model.KeyVersion, error) {
	return s.keyStore.ListAllKeys(ctx)
}

func (s *KeyRotationService) InitializeFirstKey(ctx context.Context) (*model.KeyVersion, error) {
	activeKey, err := s.keyStore.GetActiveKey(ctx)
	if err != nil && !apperrors.Is(err, store.ErrNotFound) {
		return nil, serviceutil.WrapServiceError("检查活跃密钥", err)
	}

	if activeKey != nil {
		return activeKey, nil
	}

	return s.RotateKey(ctx)
}
