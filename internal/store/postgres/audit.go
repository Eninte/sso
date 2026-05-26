// Package postgres PostgreSQL 审计日志存储实现
package postgres

import (
	"context"
	"fmt"

	"github.com/your-org/sso/internal/model"
)

// ============================================================================
// 审计日志存储实现
// ============================================================================

// StoreAuditLog 存储审计日志
func (s *Store) StoreAuditLog(ctx context.Context, log *model.AuditLog) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	query := `
		INSERT INTO audit_logs (id, event_type, user_id, client_id, ip_address, user_agent, details, success, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.db.ExecContext(ctx, query,
		log.ID,
		log.EventType,
		log.UserID,
		log.ClientID,
		log.IPAddress,
		log.UserAgent,
		log.Details,
		log.Success,
		log.Timestamp,
	)
	return err
}

// ListAuditLogs 列出审计日志（支持分页和过滤）
// 使用安全的查询构建方式，避免SQL注入风险
func (s *Store) ListAuditLogs(ctx context.Context, userID string, eventType string, offset, limit int) ([]*model.AuditLog, int, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 使用安全的查询构建器
	// 构建WHERE条件列表
	var conditions []string
	var args []interface{}

	if userID != "" {
		conditions = append(conditions, "user_id = $"+fmt.Sprint(len(args)+1))
		args = append(args, userID)
	}

	if eventType != "" {
		conditions = append(conditions, "event_type = $"+fmt.Sprint(len(args)+1))
		args = append(args, eventType)
	}

	// 构建WHERE子句
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + joinConditions(conditions, " AND ")
	}

	// 获取总数
	var total int
	countQuery := "SELECT COUNT(*) FROM audit_logs " + whereClause
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 构建分页查询
	// 添加LIMIT和OFFSET参数
	limitArgIndex := len(args) + 1
	offsetArgIndex := len(args) + 2
	args = append(args, limit, offset)

	query := "SELECT id, event_type, user_id, client_id, ip_address, user_agent, details, success, timestamp " +
		"FROM audit_logs " +
		whereClause +
		" ORDER BY timestamp DESC " +
		"LIMIT $" + fmt.Sprint(limitArgIndex) + " OFFSET $" + fmt.Sprint(offsetArgIndex)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// 预分配slice容量以减少内存重新分配
	logs := make([]*model.AuditLog, 0, limit)
	for rows.Next() {
		log := &model.AuditLog{}
		err := rows.Scan(
			&log.ID,
			&log.EventType,
			&log.UserID,
			&log.ClientID,
			&log.IPAddress,
			&log.UserAgent,
			&log.Details,
			&log.Success,
			&log.Timestamp,
		)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// joinConditions 连接SQL条件
// 这是一个辅助函数，用于安全地连接WHERE条件
func joinConditions(conditions []string, separator string) string {
	if len(conditions) == 0 {
		return ""
	}
	result := conditions[0]
	for i := 1; i < len(conditions); i++ {
		result += separator + conditions[i]
	}
	return result
}
