// Package service 审计日志服务
// 记录所有认证和授权事件
package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/your-org/sso/internal/common"
	"github.com/your-org/sso/internal/model"
	"github.com/your-org/sso/internal/store"
)

// ============================================================================
// 审计日志服务
// ============================================================================

const (
	// auditWorkerCount 审计日志worker数量
	auditWorkerCount = 5
	// auditLogChannelSize 审计日志channel缓冲区大小
	auditLogChannelSize = 1000
)

// AuditService 审计日志服务
type AuditService struct {
	store    store.Store
	logger   *slog.Logger
	logChan  chan *model.AuditLog
	stopChan chan struct{}
}

// NewAuditService 创建审计日志服务
func NewAuditService(store store.Store) *AuditService {
	s := &AuditService{
		store:    store,
		logger:   slog.Default().With("component", "audit"),
		logChan:  make(chan *model.AuditLog, auditLogChannelSize),
		stopChan: make(chan struct{}),
	}

	// 启动worker池
	s.startWorkers()

	return s
}

// startWorkers 启动worker池
func (s *AuditService) startWorkers() {
	for i := 0; i < auditWorkerCount; i++ {
		go s.worker(i)
	}
}

// worker 审计日志处理worker
func (s *AuditService) worker(id int) {
	slogger := s.logger.With("worker_id", id)
	slogger.Debug("审计日志worker启动")

	for {
		select {
		case <-s.stopChan:
			slogger.Debug("审计日志worker停止")
			return
		case log := <-s.logChan:
			if err := s.store.StoreAuditLog(context.Background(), log); err != nil {
				slogger.Error("存储审计日志失败", "error", err, "log_id", log.ID)
			}
		}
	}
}

// Close 关闭审计日志服务
func (s *AuditService) Close() {
	close(s.stopChan)
}

// Log 记录审计日志
func (s *AuditService) Log(ctx context.Context, log *model.AuditLog) {
	// 生成日志ID
	if log.ID == "" {
		log.ID = generateAuditID()
	}

	// 设置时间戳
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	// 序列化详情用于日志输出
	detailsJSON, _ := json.Marshal(log.Details)

	// 记录到slog
	s.logger.InfoContext(ctx, "审计事件",
		"id", log.ID,
		"event_type", log.EventType,
		"user_id", log.UserID,
		"client_id", log.ClientID,
		"ip_address", log.IPAddress,
		"user_agent", log.UserAgent,
		"details", string(detailsJSON),
		"success", log.Success,
		"timestamp", log.Timestamp,
	)

	// 异步存储到数据库（通过worker池，不阻塞主流程）
	select {
	case s.logChan <- log:
		// 成功发送到channel
	default:
		// channel已满，记录警告但不阻塞
		s.logger.Warn("审计日志channel已满，丢弃日志", "log_id", log.ID)
	}
}

// LogUserRegister 记录用户注册事件
func (s *AuditService) LogUserRegister(ctx context.Context, userID, email, ipAddress string, success bool) {
	details, _ := json.Marshal(map[string]interface{}{"email": email})
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventUserRegister),
		UserID:    userID,
		IPAddress: ipAddress,
		Details:   string(details),
		Success:   success,
	})
}

// LogUserLogin 记录用户登录事件
func (s *AuditService) LogUserLogin(ctx context.Context, userID, email, ipAddress, userAgent string, success bool) {
	details, _ := json.Marshal(map[string]interface{}{"email": email})
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventUserLogin),
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Details:   string(details),
		Success:   success,
	})
}

// LogTokenIssued 记录Token签发事件
func (s *AuditService) LogTokenIssued(ctx context.Context, userID, clientID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventTokenIssued),
		UserID:    userID,
		ClientID:  clientID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

// LogAuthCodeCreated 记录授权码创建事件
func (s *AuditService) LogAuthCodeCreated(ctx context.Context, userID, clientID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventAuthCodeCreated),
		UserID:    userID,
		ClientID:  clientID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// generateAuditID 生成审计日志ID
func generateAuditID() string {
	randomStr, err := common.GenerateRandomString(8)
	if err != nil {
		// 如果生成随机字符串失败，使用时间戳作为fallback
		return time.Now().Format("20060102150405") + "-fallback"
	}
	return time.Now().Format("20060102150405") + "-" + randomStr
}
