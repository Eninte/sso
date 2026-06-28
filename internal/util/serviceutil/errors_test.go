// Package serviceutil_test 测试 serviceutil 包
package serviceutil_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/serviceutil"
)

// ============================================================================
// HandleStoreError 测试
// ============================================================================

func TestHandleStoreError_ErrNotFound(t *testing.T) {
	t.Parallel()

	// 测试 ErrNotFound 映射到指定的 notFoundErr
	err := serviceutil.HandleStoreError(store.ErrNotFound, apperrors.ErrInvalidCredentials)

	assert.ErrorIs(t, err, apperrors.ErrInvalidCredentials)
	assert.Equal(t, apperrors.ErrInvalidCredentials, err)
}

func TestHandleStoreError_ErrDuplicateEmail(t *testing.T) {
	t.Parallel()

	// 测试 ErrDuplicateEmail 直接返回（保持原始错误语义）
	err := serviceutil.HandleStoreError(store.ErrDuplicateEmail, apperrors.ErrInvalidCredentials)

	assert.ErrorIs(t, err, store.ErrDuplicateEmail)
	assert.Equal(t, store.ErrDuplicateEmail, err)
}

func TestHandleStoreError_OtherError(t *testing.T) {
	t.Parallel()

	// 测试其他错误直接返回
	originalErr := errors.New("some database error")
	err := serviceutil.HandleStoreError(originalErr, apperrors.ErrInvalidCredentials)

	assert.Same(t, originalErr, err)
}

func TestHandleStoreError_NilError(t *testing.T) {
	t.Parallel()

	// 测试 nil 错误处理
	err := serviceutil.HandleStoreError(nil, apperrors.ErrInvalidCredentials)

	assert.Nil(t, err)
}

func TestHandleStoreError_WrappedError(t *testing.T) {
	t.Parallel()

	// 测试包装错误处理
	wrappedErr := fmt.Errorf("wrapped: %w", store.ErrNotFound)
	err := serviceutil.HandleStoreError(wrappedErr, apperrors.ErrInvalidCredentials)

	// 应该能够识别包装后的 ErrNotFound，并返回 notFoundErr
	assert.ErrorIs(t, err, apperrors.ErrInvalidCredentials)
	assert.Equal(t, apperrors.ErrInvalidCredentials, err)
}

func TestHandleStoreError_AppError(t *testing.T) {
	t.Parallel()

	// 测试 AppError 直接返回
	appErr := apperrors.New(apperrors.ErrCodeInternal, "internal error", 500)
	err := serviceutil.HandleStoreError(appErr, apperrors.ErrInvalidCredentials)

	assert.Same(t, appErr, err)
}

// ============================================================================
// WrapServiceError 测试
// ============================================================================

func TestWrapServiceError_BasicWrap(t *testing.T) {
	t.Parallel()

	// 测试基本包装
	originalErr := errors.New("database error")
	err := serviceutil.WrapServiceError("创建用户", originalErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "创建用户失败")
	assert.ErrorIs(t, err, originalErr)
}

func TestWrapServiceError_WithContext(t *testing.T) {
	t.Parallel()

	// 测试带上下文包装
	originalErr := errors.New("validation failed")
	err := serviceutil.WrapServiceError("验证邮箱", originalErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "验证邮箱失败")
	assert.ErrorIs(t, err, originalErr)
}

func TestWrapServiceError_NilError(t *testing.T) {
	t.Parallel()

	// 测试 nil 错误处理
	err := serviceutil.WrapServiceError("创建用户", nil)

	assert.Nil(t, err)
}

func TestWrapServiceError_ErrorChain(t *testing.T) {
	t.Parallel()

	// 测试错误链验证
	originalErr := errors.New("root cause")
	wrappedErr := serviceutil.WrapServiceError("操作1", originalErr)
	doubleWrappedErr := serviceutil.WrapServiceError("操作2", wrappedErr)

	assert.Error(t, doubleWrappedErr)
	assert.Contains(t, doubleWrappedErr.Error(), "操作2失败")
	assert.Contains(t, doubleWrappedErr.Error(), "操作1失败")
	assert.ErrorIs(t, doubleWrappedErr, originalErr)
}

func TestWrapServiceError_AppError(t *testing.T) {
	t.Parallel()

	// 测试 AppError 包装（保持错误语义）
	appErr := apperrors.New(apperrors.ErrCodeInvalidCredentials, "invalid credentials", 401)
	err := serviceutil.WrapServiceError("登录", appErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "登录失败")
	assert.ErrorIs(t, err, appErr)

	// 验证可以通过 As 获取原始 AppError
	var retrievedAppErr *apperrors.AppError
	assert.True(t, apperrors.As(err, &retrievedAppErr))
	assert.Equal(t, apperrors.ErrCodeInvalidCredentials, retrievedAppErr.Code)
	assert.Equal(t, 401, retrievedAppErr.HTTPStatus)
}

func TestWrapServiceError_StoreError(t *testing.T) {
	t.Parallel()

	// 测试 Store 错误包装
	err := serviceutil.WrapServiceError("查询用户", store.ErrNotFound)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "查询用户失败")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestHandleStoreError_DifferentNotFoundErrors(t *testing.T) {
	t.Parallel()

	// 测试不同的 notFoundErr 参数
	customErr := errors.New("custom not found error")
	err := serviceutil.HandleStoreError(store.ErrNotFound, customErr)

	assert.Same(t, customErr, err)
}

func TestWrapServiceError_EmptyOperation(t *testing.T) {
	t.Parallel()

	// 测试空操作描述
	originalErr := errors.New("some error")
	err := serviceutil.WrapServiceError("", originalErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "失败")
	assert.ErrorIs(t, err, originalErr)
}

func TestWrapServiceError_SpecialCharacters(t *testing.T) {
	t.Parallel()

	// 测试特殊字符
	originalErr := errors.New("error")
	err := serviceutil.WrapServiceError("创建/更新用户", originalErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "创建/更新用户失败")
}

func TestWrapServiceError_UnicodeOperation(t *testing.T) {
	t.Parallel()

	// 测试 Unicode 字符
	originalErr := errors.New("error")
	err := serviceutil.WrapServiceError("创建用户（测试）", originalErr)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "创建用户（测试）失败")
}
