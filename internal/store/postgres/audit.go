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
// 注意: SQL格式化是安全的，因为whereClause只包含固定的SQL片段
// 用户输入通过参数化查询（$1, $2...）传递，不存在SQL注入风险
func (s *Store) ListAuditLogs(ctx context.Context, userID string, eventType string, offset, limit int) ([]*model.AuditLog, int, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()

	// 构建查询条件
	whereClause := "WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	if userID != "" {
		whereClause += fmt.Sprintf(" AND user_id = $%d", argIndex)
		args = append(args, userID)
		argIndex++
	}

	if eventType != "" {
		whereClause += fmt.Sprintf(" AND event_type = $%d", argIndex)
		args = append(args, eventType)
		argIndex++
	}

	// 获取总数
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereClause)
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	query := fmt.Sprintf(`
		SELECT id, event_type, user_id, client_id, ip_address, user_agent, details, success, timestamp
		FROM audit_logs
		%s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

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
