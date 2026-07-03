// Package store 数据存储层接口测试
//
// 验证接口定义和错误变量的正确性
package store

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apperrors "github.com/example/sso/internal/errors"
)

// TestErrorVariables 验证错误变量正确定义
func TestErrorVariables(t *testing.T) {
	t.Parallel()

	// ErrNotFound 应等于 apperrors.ErrNotFound
	assert.Equal(t, apperrors.ErrNotFound, ErrNotFound)

	// ErrDuplicateEmail 应等于 apperrors.ErrEmailExists
	assert.Equal(t, apperrors.ErrEmailExists, ErrDuplicateEmail)

	// ErrDuplicateClient 应等于 apperrors.ErrConflict
	assert.Equal(t, apperrors.ErrConflict, ErrDuplicateClient)

	// ErrAuthorizationCodeUsed 应等于 apperrors.ErrCodeUsedErr
	assert.Equal(t, apperrors.ErrCodeUsedErr, ErrAuthorizationCodeUsed)
}
