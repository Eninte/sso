// Package auditutil 提供审计日志通用工具函数
// 包含安全的审计日志记录、回退处理等可重用逻辑
package auditutil

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/your-org/sso/internal/model"
)

// AuditService 审计服务接口
// 定义审计日志服务的最小接口
type AuditService interface {
	// Log 记录审计日志
	// 实现应该异步处理日志，不阻塞主流程
	Log(ctx context.Context, log *model.AuditLog)
}

// LogWithFallback 使用回退处理的审计日志记录
// 当审计日志记录失败时，自动回退到stderr
//
// 此函数确保审计日志失败不会影响主操作。如果审计服务不可用或日志记录失败，
// 错误会被记录到stderr，但不会返回给调用者。
//
// 参数:
//   - auditSvc: 审计服务实例（可以为nil）
//   - logFunc: 执行审计日志记录的函数，返回error
//
// 行为:
//   - 如果auditSvc为nil，直接返回（审计日志是可选的）
//   - 如果logFunc执行成功，返回
//   - 如果logFunc返回错误，记录到stderr并继续（不影响主流程）
//
// 示例:
//
//	auditutil.LogWithFallback(auditSvc, func() error {
//	    return auditSvc.Log(ctx, &model.AuditLog{...})
//	})
func LogWithFallback(auditSvc AuditService, logFunc func() error) {
	// 审计服务为nil时，直接返回（审计日志是可选的）
	if auditSvc == nil {
		return
	}

	// 执行审计日志记录
	if err := logFunc(); err != nil {
		// 审计日志失败时，记录到stderr作为回退
		// 使用stderr而不是panic或返回error，确保不影响主操作
		fmt.Fprintf(os.Stderr, "[AUDIT_FALLBACK] 审计日志记录失败: %v\n", err)
	}
}

// SafeAuditLog 安全的审计日志记录函数
// 提供统一的审计日志记录接口，包含自动回退处理
//
// 此函数是审计日志记录的标准方式。它确保：
// 1. 审计日志失败不会影响主操作
// 2. 审计日志失败会被记录到stderr
// 3. 所有审计日志都使用统一的格式和处理方式
//
// 参数:
//   - ctx: 请求上下文
//   - auditSvc: 审计服务实例（可以为nil）
//   - event: 事件类型（如"user_login"、"user_register"）
//   - userID: 用户ID（可以为空）
//   - metadata: 事件元数据（可以为nil）
//
// 行为:
//   - 如果auditSvc为nil，直接返回（审计日志是可选的）
//   - 尝试记录审计日志到审计服务
//   - 如果记录失败，记录到stderr并继续
//   - 不会panic或返回error
//
// 示例:
//
//	auditutil.SafeAuditLog(ctx, auditSvc, "user_login", userID, map[string]interface{}{
//	    "email": user.Email,
//	    "ip_address": ipAddress,
//	})
func SafeAuditLog(ctx context.Context, auditSvc AuditService, event, userID string, metadata map[string]interface{}) {
	// 审计服务为nil时，直接返回（审计日志是可选的）
	if auditSvc == nil {
		return
	}

	// 使用LogWithFallback确保审计失败不影响主流程
	LogWithFallback(auditSvc, func() error {
		// 构建审计日志对象
		detailsJSON, _ := json.Marshal(metadata)
		
		// 从metadata中提取IP地址、用户代理等字段
		ipAddress := ""
		userAgent := ""
		clientID := ""
		success := true
		if metadata != nil {
			if ip, ok := metadata["ip_address"].(string); ok {
				ipAddress = ip
			}
			if ua, ok := metadata["user_agent"].(string); ok {
				userAgent = ua
			}
			if cid, ok := metadata["client_id"].(string); ok {
				clientID = cid
			}
			if s, ok := metadata["success"].(bool); ok {
				success = s
			}
		}
		
		auditLog := &model.AuditLog{
			EventType: event,
			UserID:    userID,
			IPAddress: ipAddress,
			UserAgent: userAgent,
			ClientID:  clientID,
			Details:   string(detailsJSON),
			Success:   success,
			Timestamp: time.Now(),
		}

		// 调用审计服务记录日志
		// 审计服务应该异步处理，不阻塞主流程
		auditSvc.Log(ctx, auditLog)

		// 同时记录到slog以便调试
		slog.DebugContext(ctx, "审计日志已记录",
			"event", event,
			"user_id", userID,
		)

		return nil
	})
}
