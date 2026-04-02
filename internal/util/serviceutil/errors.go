// Package serviceutil 提供服务层通用工具函数
// 包含错误处理、数据转换等可重用逻辑
package serviceutil

import (
	"fmt"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/store"
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
//   - 否则返回原始错误（保持错误语义）
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

	// 保持原始错误语义
	return err
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
//   - 如果err是AppError，保持其错误码和HTTP状态，只添加上下文
//   - 否则包装为通用错误
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

	// 如果是AppError，保持其错误语义
	var appErr *apperrors.AppError
	if apperrors.As(err, &appErr) {
		// 保持原始错误码和HTTP状态，只添加操作上下文
		return fmt.Errorf("%s失败: %w", operation, err)
	}

	// 对于非AppError，包装为通用错误
	return fmt.Errorf("%s失败: %w", operation, err)
}
