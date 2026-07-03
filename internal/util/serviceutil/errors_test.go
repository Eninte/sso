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

	// 测试 ErrDuplicateEmail 是 AppError，应保持原始错误语义
	err := serviceutil.HandleStoreError(store.ErrDuplicateEmail, apperrors.ErrInvalidCredentials)

	assert.ErrorIs(t, err, store.ErrDuplicateEmail)
	assert.Equal(t, store.ErrDuplicateEmail, err)
}

func TestHandleStoreError_OtherError(t *testing.T) {
	t.Parallel()

	// 测试其他非 AppError 错误映射为 ErrInternal，不暴露原始详情
	originalErr := errors.New("some database error")
	err := serviceutil.HandleStoreError(originalErr, apperrors.ErrInvalidCredentials)

	assert.Error(t, err)
	assert.NotSame(t, originalErr, err)
	assert.NotContains(t, err.Error(), "some database error")

	// 验证返回的是 ErrInternal
	var appErr *apperrors.AppError
	assert.True(t, apperrors.As(err, &appErr))
	assert.Equal(t, apperrors.ErrCodeInternal, appErr.Code)
	assert.Equal(t, 500, appErr.HTTPStatus)
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

	// 测试 AppError 直接返回（保持语义）
	appErr := apperrors.New(apperrors.ErrCodeInternal, "internal error", 500)
	err := serviceutil.HandleStoreError(appErr, apperrors.ErrInvalidCredentials)

	assert.Same(t, appErr, err)
}

// ============================================================================
// WrapServiceError 测试
// ============================================================================

func TestWrapServiceError_BasicWrap(t *testing.T) {
	t.Parallel()

	// 测试非 AppError 错误映射为 ErrInternal，不暴露原始详情
	originalErr := errors.New("database error")
	err := serviceutil.WrapServiceError("创建用户", originalErr)

	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "创建用户")
	assert.NotContains(t, err.Error(), "database error")

	// 验证返回的是 ErrInternal
	var appErr *apperrors.AppError
	assert.True(t, apperrors.As(err, &appErr))
	assert.Equal(t, apperrors.ErrCodeInternal, appErr.Code)
}

func TestWrapServiceError_NilError(t *testing.T) {
	t.Parallel()

	// 测试 nil 错误处理
	err := serviceutil.WrapServiceError("创建用户", nil)

	assert.Nil(t, err)
}

func TestWrapServiceError_AppError(t *testing.T) {
	t.Parallel()

	// 测试 AppError 保持错误语义（直接返回，不添加操作上下文）
	appErr := apperrors.New(apperrors.ErrCodeInvalidCredentials, "invalid credentials", 401)
	err := serviceutil.WrapServiceError("登录", appErr)

	assert.Error(t, err)
	assert.ErrorIs(t, err, appErr)

	// 验证可以通过 As 获取原始 AppError
	var retrievedAppErr *apperrors.AppError
	assert.True(t, apperrors.As(err, &retrievedAppErr))
	assert.Equal(t, apperrors.ErrCodeInvalidCredentials, retrievedAppErr.Code)
	assert.Equal(t, 401, retrievedAppErr.HTTPStatus)
}

func TestWrapServiceError_StoreError(t *testing.T) {
	t.Parallel()

	// 测试 Store 的 AppError 错误保持语义
	err := serviceutil.WrapServiceError("查询用户", store.ErrNotFound)

	assert.Error(t, err)
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

	// 测试空操作描述（非 AppError 仍映射为 ErrInternal）
	originalErr := errors.New("some error")
	err := serviceutil.WrapServiceError("", originalErr)

	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "some error")

	var appErr *apperrors.AppError
	assert.True(t, apperrors.As(err, &appErr))
	assert.Equal(t, apperrors.ErrCodeInternal, appErr.Code)
}
