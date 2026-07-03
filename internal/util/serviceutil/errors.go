// Package serviceutil 提供服务层通用工具函数
// 包含错误处理、数据转换等可重用逻辑
package serviceutil

import (
	"fmt"
	"log/slog"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/store"
)

// HandleStoreError 处理store层错误并映射到service层错误
// 保持错误语义（类型、代码、消息）
//
// 参数:
//   - err: store层返回的错误
//   - notFoundErr: 当store返回ErrNotFound时应该返回的错误（如ErrInvalidCredentials）
//
// 返回:
//   - 如果err为nil，返回nil
//   - 如果err是store.ErrNotFound，返回notFoundErr
//   - 如果err已经是AppError，保持其语义
//   - 否则映射为ErrInternal，不暴露原始数据库错误详情（原始错误仅记录日志）
//
// 示例:
//
//	user, err := s.store.GetByEmail(ctx, email)
//	if err != nil {
//	    return nil, serviceutil.HandleStoreError(err, ErrInvalidCredentials)
//	}
func HandleStoreError(err error, notFoundErr error) error {
	if err == nil {
		return nil
	}

	// 如果是NotFound错误，返回指定的notFoundErr
	if apperrors.Is(err, store.ErrNotFound) {
		return notFoundErr
	}

	// 如果已经是AppError，保持其语义
	var appErr *apperrors.AppError
	if apperrors.As(err, &appErr) {
		return err
	}

	// 非预期的Store错误，记录原始错误用于调试，返回不包含内部详情的ErrInternal
	slog.Error("internal store error", "error", err)
	return apperrors.New(apperrors.ErrCodeInternal, "internal service error", 500)
}

// WrapServiceError 包装service层错误，添加操作上下文
// 保持错误语义（类型、代码、消息），只添加上下文信息
//
// 参数:
//   - operation: 操作描述（如"创建用户"、"验证密码"）
//   - err: 原始错误
//
// 返回:
//   - 如果err为nil，返回nil
//   - 如果err是AppError，保持其错误码和HTTP状态，附加操作上下文
//   - 否则映射为ErrInternal，不暴露原始数据库错误详情（原始错误仅记录日志）
//
// 示例:
//
//	if err := s.store.Create(ctx, user); err != nil {
//	    return nil, serviceutil.WrapServiceError("创建用户", err)
//	}
func WrapServiceError(operation string, err error) error {
	if err == nil {
		return nil
	}

	// 如果是AppError，保持其错误码和HTTP状态，附加操作上下文
	var appErr *apperrors.AppError
	if apperrors.As(err, &appErr) {
		return fmt.Errorf("%s: %w", operation, err)
	}

	// 非预期的非AppError错误，记录原始错误用于调试，返回不包含内部详情的ErrInternal
	slog.Error("internal service error", "operation", operation, "error", err)
	return apperrors.New(apperrors.ErrCodeInternal, "internal service error", 500)
}
