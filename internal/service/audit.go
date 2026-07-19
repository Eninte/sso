// Package service 审计日志服务
// 记录所有认证和授权事件
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/example/sso/internal/common"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/safego"
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
	wg       sync.WaitGroup
	closed   atomic.Bool // 标记服务是否已关闭，防止 Close 后 Log 向已关闭 channel 发送 panic
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
		s.wg.Add(1)
		go s.worker(i)
	}
}

// worker 审计日志处理worker
func (s *AuditService) worker(id int) {
	defer s.wg.Done()
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
// 使用 atomic.Bool 的 CompareAndSwap 保证只关闭一次，防止重复 close panic
func (s *AuditService) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		// 已关闭，直接返回，避免重复 close panic
		return
	}
	close(s.stopChan)
	s.wg.Wait()
	close(s.logChan)
}

// Log 记录审计日志
func (s *AuditService) Log(ctx context.Context, log *model.AuditLog) {
	// 检查服务是否已关闭，避免向已关闭 channel 发送 panic
	if s.closed.Load() {
		// 服务已关闭，降级到 stderr，确保审计事件不丢失
		fmt.Fprintf(os.Stderr, "[AUDIT_CLOSED] 审计服务已关闭，事件降级到stderr: %s user=%s\n", log.EventType, log.UserID)
		return
	}

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
		// channel已满，降级处理：尝试同步存储或记录到slog
		s.fallbackLog(ctx, log)
	}
}

// LogSync 同步记录审计日志（用于关键事件）
// 与 Log 不同，此方法同步存储到数据库并返回 error
// 关键事件（如密码修改、MFA禁用）应使用此方法，确保审计日志不丢失
// 实现 auditutil.SyncAuditLogger 接口
func (s *AuditService) LogSync(ctx context.Context, log *model.AuditLog) error {
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
	s.logger.InfoContext(ctx, "关键审计事件（同步）",
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

	// 同步存储到数据库，返回 error
	// 关键事件必须确保日志写入存储，失败时调用者可回滚操作
	return s.store.StoreAuditLog(ctx, log)
}

// fallbackLog 降级日志处理
// 当异步channel满时，尝试同步存储或记录详细日志
func (s *AuditService) fallbackLog(ctx context.Context, log *model.AuditLog) {
	s.logger.WarnContext(ctx, "审计日志channel已满，降级处理",
		"log_id", log.ID,
		"event_type", log.EventType,
		"user_id", log.UserID,
	)

	// 尝试同步存储（带超时）
	safego.Go(slog.Default(), "降级存储审计日志", func() { //nolint:contextcheck // 异步 goroutine 必须用独立 context，请求 ctx 会随请求结束而取消 // #nosec G118 -- goroutine需要独立context，不能使用请求context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.store.StoreAuditLog(ctx, log); err != nil {
			s.logger.ErrorContext(ctx, "降级存储审计日志失败",
				"error", err,
				"log_id", log.ID,
				"event_type", log.EventType,
			)
		}
	})
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

// LogAuthCodeUsed 记录授权码使用事件
func (s *AuditService) LogAuthCodeUsed(ctx context.Context, userID, clientID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventAuthCodeUsed),
		UserID:    userID,
		ClientID:  clientID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

// LogAuthCodeInvalid 记录授权码无效事件
func (s *AuditService) LogAuthCodeInvalid(ctx context.Context, userID, clientID, ipAddress, reason string) {
	details, _ := json.Marshal(map[string]interface{}{"reason": reason})
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventAuthCodeInvalid),
		UserID:    userID,
		ClientID:  clientID,
		IPAddress: ipAddress,
		Details:   string(details),
		Success:   false,
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// generateAuditID 生成审计日志ID
func generateAuditID() string {
	randomStr, err := common.GenerateRandomString(8)
	if err != nil {
		return time.Now().Format("20060102150405") + "-fallback"
	}
	return time.Now().Format("20060102150405") + "-" + randomStr
}

func (s *AuditService) LogUserLogout(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventUserLogout),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogTokenRefresh(ctx context.Context, userID, clientID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventTokenRefresh),
		UserID:    userID,
		ClientID:  clientID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogTokenRevoke(ctx context.Context, userID, clientID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventTokenRevoke),
		UserID:    userID,
		ClientID:  clientID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

// LogUserLoginFailed 记录用户登录失败事件
func (s *AuditService) LogUserLoginFailed(ctx context.Context, userID, email, ipAddress, userAgent, reason string) {
	details, _ := json.Marshal(map[string]interface{}{"email": email, "reason": reason})
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventUserLoginFailed),
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Details:   string(details),
		Success:   false,
	})
}

func (s *AuditService) LogLogoutAll(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventLogoutAll),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogPasswordChanged(ctx context.Context, userID, ipAddress string, success bool) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventPasswordChanged),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   success,
	})
}

func (s *AuditService) LogPasswordReset(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventPasswordReset),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogAccountLocked(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventAccountLocked),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogAccountUnlocked(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventAccountUnlocked),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogMFASetup(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventMFASetup),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogMFAEnabled(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventMFAEnabled),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogMFADisabled(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventMFADisabled),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogKeyRotated(ctx context.Context, keyID string) {
	details, _ := json.Marshal(map[string]interface{}{"key_id": keyID})
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventKeyRotated),
		Details:   string(details),
		Success:   true,
	})
}

func (s *AuditService) LogKeyRevoked(ctx context.Context, keyID string) {
	details, _ := json.Marshal(map[string]interface{}{"key_id": keyID})
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventKeyRevoked),
		Details:   string(details),
		Success:   true,
	})
}

func (s *AuditService) LogSystemStart(ctx context.Context, version string) {
	details, _ := json.Marshal(map[string]interface{}{"version": version})
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventSystemStart),
		Details:   string(details),
		Success:   true,
	})
}

func (s *AuditService) LogUserDisabled(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventUserDisabled),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogUserEnabled(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventUserEnabled),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogUserDeleted(ctx context.Context, userID, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventUserDeleted),
		UserID:    userID,
		IPAddress: ipAddress,
		Success:   true,
	})
}

func (s *AuditService) LogSystemCleanup(ctx context.Context, ipAddress string) {
	s.Log(ctx, &model.AuditLog{
		EventType: string(model.EventSystemCleanup),
		IPAddress: ipAddress,
		Success:   true,
	})
}
