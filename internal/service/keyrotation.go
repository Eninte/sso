package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/logging"
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
			// 阶段 D 审查修复（H5）：keyStore 错误可能含 DSN
			slog.Warn("failed to deprecate old key", "error", logging.SanitizeDBURL(err.Error()), "key_id", activeKey.ID)
		} else {
			slog.Info("old key deprecated",
				"key_id", activeKey.ID,
				"expires_at", expiresAt,
			)
		}
	}

	// 阶段 4 安全增强：密钥轮换属于高敏感操作，使用 CriticalAuditLog 同步记录
	// 失败时返回错误供调用方决策（虽然密钥已生效无法回滚，但应明确告知调用方审计缺失）
	if err := auditutil.CriticalAuditLog(ctx, s.auditSvc, string(model.EventKeyRotated), "", map[string]interface{}{
		"key_id": newKeyVersion.ID,
	}); err != nil {
		// 密钥已生效无法回滚，但记录审计失败错误，便于运维追溯
		// 阶段 D 审查修复（H5）：audit 错误底层为 store 错误，可能含 DSN
		slog.Error("密钥轮换审计日志记录失败（密钥已生效，无法回滚）",
			"error", logging.SanitizeDBURL(err.Error()),
			"key_id", newKeyVersion.ID,
		)
		return nil, apperrors.Wrap(apperrors.ErrCodeInternal, "密钥轮换审计日志记录失败", 500, err)
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
		return 0, serviceutil.WrapServiceError("列出密钥", err)
	}

	cleanedCount := 0
	now := time.Now()

	for _, key := range keys {
		if key.Status == model.KeyStatusDeprecated && key.ExpiresAt != nil && key.ExpiresAt.Before(now) {
			if err := s.keyStore.RevokeKey(ctx, key.ID); err != nil {
				// 阶段 D 审查修复（H5）：keyStore 错误可能含 DSN
				slog.Warn("failed to revoke expired key", "error", logging.SanitizeDBURL(err.Error()), "key_id", key.ID)
				continue
			}
			s.jwtSvc.RemoveKey(key.ID)
			cleanedCount++
			slog.Info("revoked expired key", "key_id", key.ID)

			// 阶段 4 安全增强：密钥撤销使用 CriticalAuditLog 同步记录
			// 失败时返回错误，调用方可决定是否继续清理后续密钥
			if err := auditutil.CriticalAuditLog(ctx, s.auditSvc, string(model.EventKeyRevoked), "", map[string]interface{}{
				"key_id": key.ID,
			}); err != nil {
				// 阶段 D 审查修复（H5）：audit 错误底层为 store 错误，可能含 DSN
				slog.Error("密钥撤销审计日志记录失败",
					"error", logging.SanitizeDBURL(err.Error()),
					"key_id", key.ID,
				)
				return cleanedCount, apperrors.Wrap(apperrors.ErrCodeInternal, "密钥撤销审计日志记录失败", 500, err)
			}
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

	// 阶段 4 安全增强：密钥撤销使用 CriticalAuditLog 同步记录
	// 失败时返回错误供调用方决策
	if err := auditutil.CriticalAuditLog(ctx, s.auditSvc, string(model.EventKeyRevoked), "", map[string]interface{}{
		"key_id": keyID,
	}); err != nil {
		// 阶段 D 审查修复（H5）：audit 错误底层为 store 错误，可能含 DSN
		slog.Error("密钥撤销审计日志记录失败",
			"error", logging.SanitizeDBURL(err.Error()),
			"key_id", keyID,
		)
		return apperrors.Wrap(apperrors.ErrCodeInternal, "密钥撤销审计日志记录失败", 500, err)
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
		return nil, serviceutil.WrapServiceError("检查活跃密钥", err)
	}

	if activeKey != nil {
		return activeKey, nil
	}

	return s.RotateKey(ctx)
}
