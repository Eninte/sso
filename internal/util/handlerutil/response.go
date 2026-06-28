// Package handlerutil 提供HTTP响应处理通用工具函数
// 包含标准化的JSON响应格式化、错误映射等可重用逻辑
package handlerutil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	apperrors "github.com/example/sso/internal/errors"
)

// ============================================================================
// 响应格式定义
// ============================================================================

// ErrorResponse 标准化的错误响应格式
type ErrorResponse struct {
	Error   string `json:"error"`             // 错误码常量
	Message string `json:"message,omitempty"` // 错误消息（可选）
	Details string `json:"details,omitempty"` // 详细信息（可选）
}

// SuccessResponse 标准化的成功响应格式
type SuccessResponse struct {
	Data interface{} `json:"data,omitempty"` // 响应数据（可选）
}

// ValidationErrorResponse 标准化的验证错误响应格式
type ValidationErrorResponse struct {
	Error string `json:"error"`   // 错误码常量
	Field string `json:"field"`   // 字段名
	Value string `json:"message"` // 错误消息
}

// ============================================================================
// 响应写入函数
// ============================================================================

// WriteJSONError 写入标准化的错误响应
// 使用apperrors包进行错误到HTTP状态码和错误码的映射
//
// 此函数是处理错误响应的标准方式。它确保：
// 1. 所有错误响应使用统一的JSON格式
// 2. HTTP状态码通过apperrors包正确映射
// 3. 错误码常量用于客户端处理
// 4. 错误消息不暴露内部实现细节
//
// 参数:
//   - w: HTTP响应写入器
//   - err: 要处理的错误（可以是*apperrors.AppError或其他error）
//
// 行为:
//   - 如果err是*apperrors.AppError，使用其HTTP状态码和错误码
//   - 如果err是其他类型，返回500 Internal Server Error
//   - 响应体为JSON格式的ErrorResponse
//   - 自动设置Content-Type为application/json
//
// 示例:
//
//	if err != nil {
//	    handlerutil.WriteJSONError(w, err)
//	    return
//	}
func WriteJSONError(w http.ResponseWriter, err error) {
	// 获取HTTP状态码和错误码
	status := apperrors.GetHTTPStatus(err)
	code := apperrors.GetErrorCode(err)

	// 构建错误响应
	response := ErrorResponse{
		Error: string(code),
	}

	// 如果是AppError，可以添加消息和详情
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		response.Message = appErr.Message
		if appErr.Details != "" {
			response.Details = appErr.Details
		}
	}

	// 写入JSON响应
	writeJSON(w, status, response)
}

// WriteJSON 写入任意JSON响应
// 用于需要自定义状态码和响应体的场景
//
// 此函数是处理非标准成功/错误响应的底层方式。它确保：
// 1. 自动设置Content-Type为application/json
// 2. 编码错误会被记录
// 3. 状态码和响应体由调用者控制
//
// 参数:
//   - w: HTTP响应写入器
//   - status: HTTP状态码
//   - data: 要序列化的响应体
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(w, status, data)
}

// WriteJSONSuccess 写入标准化的成功响应
// 用于返回成功的操作结果
//
// 此函数是处理成功响应的标准方式。它确保：
// 1. 所有成功响应使用统一的JSON格式
// 2. HTTP状态码为200 OK（或其他2xx状态码）
// 3. 响应数据包含在data字段中
// 4. 如果data为nil，data字段会被省略
//
// 参数:
//   - w: HTTP响应写入器
//   - data: 要返回的数据（可以为nil）
//
// 行为:
//   - 返回200 OK状态码
//   - 响应体为JSON格式的SuccessResponse
//   - 自动设置Content-Type为application/json
//   - 如果data为nil，响应体为{"data":null}或{}
//
// 示例:
//
//	user := &model.User{ID: 1, Email: "test@example.com"}
//	handlerutil.WriteJSONSuccess(w, user)
//
//	// 或者不返回数据
//	handlerutil.WriteJSONSuccess(w, nil)
func WriteJSONSuccess(w http.ResponseWriter, data interface{}) {
	response := SuccessResponse{
		Data: data,
	}

	writeJSON(w, http.StatusOK, response)
}

// WriteValidationError 写入标准化的验证错误响应
// 用于处理字段级别的验证错误
//
// 此函数用于处理特定字段的验证失败。它确保：
// 1. 验证错误使用统一的JSON格式
// 2. 包含失败的字段名
// 3. 包含清晰的错误消息
// 4. HTTP状态码为400 Bad Request
//
// 参数:
//   - w: HTTP响应写入器
//   - field: 验证失败的字段名（如"email"、"password"）
//   - message: 验证错误消息（如"邮箱格式无效"）
//
// 行为:
//   - 返回400 Bad Request状态码
//   - 响应体为JSON格式的ValidationErrorResponse
//   - 自动设置Content-Type为application/json
//
// 示例:
//
//	if !isValidEmail(email) {
//	    handlerutil.WriteValidationError(w, "email", "邮箱格式无效")
//	    return
//	}
//
//	if len(password) < 8 {
//	    handlerutil.WriteValidationError(w, "password", "密码长度不能少于8个字符")
//	    return
//	}
func WriteValidationError(w http.ResponseWriter, field, message string) {
	response := ValidationErrorResponse{
		Error: string(apperrors.ErrCodeBadRequest),
		Field: field,
		Value: message,
	}

	writeJSON(w, http.StatusBadRequest, response)
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// writeJSON 写入JSON响应的内部辅助函数
// 设置Content-Type头并编码JSON响应
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// 响应头已发送，无法返回错误给客户端，记录日志以便排查
		slog.Error("编码JSON响应失败", "error", err)
	}
}
