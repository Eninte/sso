package handlerutil_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/your-org/sso/internal/errors"
	"github.com/your-org/sso/internal/util/handlerutil"
)

// ============================================================================
// WriteJSONError 测试
// ============================================================================

// TestWriteJSONError_WithAppError 测试WriteJSONError处理AppError
func TestWriteJSONError_WithAppError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	err := apperrors.ErrInvalidCredentials

	handlerutil.WriteJSONError(w, err)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var response handlerutil.ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, string(apperrors.ErrCodeInvalidCredentials), response.Error)
	assert.Equal(t, "邮箱或密码错误", response.Message)
}

// TestWriteJSONError_WithEmailExists 测试WriteJSONError处理邮箱已存在错误
func TestWriteJSONError_WithEmailExists(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	err := apperrors.ErrEmailExists

	handlerutil.WriteJSONError(w, err)

	assert.Equal(t, http.StatusConflict, w.Code)

	var response handlerutil.ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, string(apperrors.ErrCodeEmailExists), response.Error)
	assert.Equal(t, "邮箱已注册", response.Message)
}

// TestWriteJSONError_WithAccountLocked 测试WriteJSONError处理账户锁定错误
func TestWriteJSONError_WithAccountLocked(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	err := apperrors.ErrAccountLocked

	handlerutil.WriteJSONError(w, err)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response handlerutil.ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, string(apperrors.ErrCodeAccountLocked), response.Error)
}

// TestWriteJSONError_WithDetails 测试WriteJSONError包含详情
func TestWriteJSONError_WithDetails(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	// 创建新的错误实例而不是修改共享的预定义错误
	err := apperrors.New(apperrors.ErrCodeInvalidCredentials, "邮箱或密码错误", http.StatusUnauthorized)
	err = err.WithDetails("用户不存在")

	handlerutil.WriteJSONError(w, err)

	var response handlerutil.ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "用户不存在", response.Details)
}

// TestWriteJSONError_WithGenericError 测试WriteJSONError处理通用错误
func TestWriteJSONError_WithGenericError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	err := apperrors.New(apperrors.ErrCodeInternal, "数据库连接失败", http.StatusInternalServerError)

	handlerutil.WriteJSONError(w, err)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response handlerutil.ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, string(apperrors.ErrCodeInternal), response.Error)
	assert.Equal(t, "数据库连接失败", response.Message)
}

// TestWriteJSONError_WithNonAppError 测试WriteJSONError处理非AppError
func TestWriteJSONError_WithNonAppError(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	err := apperrors.New(apperrors.ErrCodeInternal, "未知错误", http.StatusInternalServerError)

	handlerutil.WriteJSONError(w, err)

	// 应该返回500状态码
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response handlerutil.ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, string(apperrors.ErrCodeInternal), response.Error)
}

// TestWriteJSONError_StatusCodeMapping 测试WriteJSONError的状态码映射
func TestWriteJSONError_StatusCodeMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   apperrors.ErrorCode
	}{
		{
			name:       "InvalidCredentials",
			err:        apperrors.ErrInvalidCredentials,
			wantStatus: http.StatusUnauthorized,
			wantCode:   apperrors.ErrCodeInvalidCredentials,
		},
		{
			name:       "EmailExists",
			err:        apperrors.ErrEmailExists,
			wantStatus: http.StatusConflict,
			wantCode:   apperrors.ErrCodeEmailExists,
		},
		{
			name:       "AccountLocked",
			err:        apperrors.ErrAccountLocked,
			wantStatus: http.StatusForbidden,
			wantCode:   apperrors.ErrCodeAccountLocked,
		},
		{
			name:       "NotFound",
			err:        apperrors.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   apperrors.ErrCodeNotFound,
		},
		{
			name:       "BadRequest",
			err:        apperrors.ErrBadRequest,
			wantStatus: http.StatusBadRequest,
			wantCode:   apperrors.ErrCodeBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			handlerutil.WriteJSONError(w, tt.err)

			assert.Equal(t, tt.wantStatus, w.Code)

			var response handlerutil.ErrorResponse
			unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, unmarshalErr)

			assert.Equal(t, string(tt.wantCode), response.Error)
		})
	}
}

// ============================================================================
// WriteJSONSuccess 测试
// ============================================================================

// TestWriteJSONSuccess_WithData 测试WriteJSONSuccess返回数据
func TestWriteJSONSuccess_WithData(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"id":    1,
		"email": "test@example.com",
	}

	handlerutil.WriteJSONSuccess(w, data)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var response handlerutil.SuccessResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	// 验证data字段包含返回的数据
	assert.NotNil(t, response.Data)
}

// TestWriteJSONSuccess_WithNilData 测试WriteJSONSuccess返回nil数据
func TestWriteJSONSuccess_WithNilData(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	handlerutil.WriteJSONSuccess(w, nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var response handlerutil.SuccessResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	// data字段应该为nil
	assert.Nil(t, response.Data)
}

// TestWriteJSONSuccess_WithStruct 测试WriteJSONSuccess返回结构体
func TestWriteJSONSuccess_WithStruct(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	type User struct {
		ID    int    `json:"id"`
		Email string `json:"email"`
	}

	user := User{ID: 1, Email: "test@example.com"}
	handlerutil.WriteJSONSuccess(w, user)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	// 验证data字段包含用户数据
	assert.NotNil(t, response["data"])
}

// TestWriteJSONSuccess_WithArray 测试WriteJSONSuccess返回数组
func TestWriteJSONSuccess_WithArray(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	data := []map[string]interface{}{
		{"id": 1, "email": "user1@example.com"},
		{"id": 2, "email": "user2@example.com"},
	}

	handlerutil.WriteJSONSuccess(w, data)

	assert.Equal(t, http.StatusOK, w.Code)

	var response handlerutil.SuccessResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.NotNil(t, response.Data)
}

// ============================================================================
// WriteValidationError 测试
// ============================================================================

// TestWriteValidationError_EmailField 测试WriteValidationError处理邮箱字段错误
func TestWriteValidationError_EmailField(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	handlerutil.WriteValidationError(w, "email", "邮箱格式无效")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var response handlerutil.ValidationErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, string(apperrors.ErrCodeBadRequest), response.Error)
	assert.Equal(t, "email", response.Field)
	assert.Equal(t, "邮箱格式无效", response.Value)
}

// TestWriteValidationError_PasswordField 测试WriteValidationError处理密码字段错误
func TestWriteValidationError_PasswordField(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	handlerutil.WriteValidationError(w, "password", "密码长度不能少于8个字符")

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response handlerutil.ValidationErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "password", response.Field)
	assert.Equal(t, "密码长度不能少于8个字符", response.Value)
}

// TestWriteValidationError_MultipleFields 测试WriteValidationError处理多个字段
func TestWriteValidationError_MultipleFields(t *testing.T) {
	t.Parallel()
	fields := []struct {
		field   string
		message string
	}{
		{"email", "邮箱不能为空"},
		{"password", "密码不能为空"},
		{"username", "用户名格式无效"},
	}

	for _, f := range fields {
		w := httptest.NewRecorder()
		handlerutil.WriteValidationError(w, f.field, f.message)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response handlerutil.ValidationErrorResponse
		unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, unmarshalErr)

		assert.Equal(t, f.field, response.Field)
		assert.Equal(t, f.message, response.Value)
	}
}

// ============================================================================
// 集成测试
// ============================================================================

// TestWriteJSONError_ContentType 测试WriteJSONError设置正确的Content-Type
func TestWriteJSONError_ContentType(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	err := apperrors.ErrInvalidCredentials

	handlerutil.WriteJSONError(w, err)

	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
}

// TestWriteJSONSuccess_ContentType 测试WriteJSONSuccess设置正确的Content-Type
func TestWriteJSONSuccess_ContentType(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	handlerutil.WriteJSONSuccess(w, map[string]string{"key": "value"})

	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
}

// TestWriteValidationError_ContentType 测试WriteValidationError设置正确的Content-Type
func TestWriteValidationError_ContentType(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	handlerutil.WriteValidationError(w, "email", "邮箱格式无效")

	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
}

// TestWriteJSONError_ValidJSON 测试WriteJSONError返回有效的JSON
func TestWriteJSONError_ValidJSON(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	err := apperrors.ErrInvalidCredentials

	handlerutil.WriteJSONError(w, err)

	// 验证响应体是有效的JSON
	var response handlerutil.ErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)
}

// TestWriteJSONSuccess_ValidJSON 测试WriteJSONSuccess返回有效的JSON
func TestWriteJSONSuccess_ValidJSON(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	handlerutil.WriteJSONSuccess(w, map[string]string{"key": "value"})

	// 验证响应体是有效的JSON
	var response handlerutil.SuccessResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)
}

// TestWriteValidationError_ValidJSON 测试WriteValidationError返回有效的JSON
func TestWriteValidationError_ValidJSON(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()

	handlerutil.WriteValidationError(w, "email", "邮箱格式无效")

	// 验证响应体是有效的JSON
	var response handlerutil.ValidationErrorResponse
	unmarshalErr := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, unmarshalErr)
}
