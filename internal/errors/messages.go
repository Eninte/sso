// Package errors 错误消息国际化
// 提供多语言错误消息支持
package errors

import (
	"embed"
	"encoding/json"
	"sync"

	"github.com/example/sso/internal/common"
)

// ============================================================================
// 嵌入语言文件
// ============================================================================

//go:embed locales/*.json
var localeFiles embed.FS

// ============================================================================
// 消息管理器
// ============================================================================

// messageManager 消息管理器
type messageManager struct {
	messages map[string]map[ErrorCode]string // lang -> ErrorCode -> message
	mu       sync.RWMutex
}

// globalMessageManager 全局消息管理器
var globalMessageManager = &messageManager{
	messages: make(map[string]map[ErrorCode]string),
}

// init 初始化消息管理器
func init() {
	// 加载所有语言文件
	languages := []string{"zh-CN", "en-US"}
	for _, lang := range languages {
		if err := loadMessages(lang); err != nil {
			// 如果加载失败，使用空map
			globalMessageManager.messages[lang] = make(map[ErrorCode]string)
		}
	}
}

// loadMessages 加载指定语言的消息
func loadMessages(lang string) error {
	data, err := localeFiles.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return err
	}

	var messages map[string]string
	if err := json.Unmarshal(data, &messages); err != nil {
		return err
	}

	globalMessageManager.mu.Lock()
	defer globalMessageManager.mu.Unlock()

	if globalMessageManager.messages[lang] == nil {
		globalMessageManager.messages[lang] = make(map[ErrorCode]string)
	}

	for code, msg := range messages {
		globalMessageManager.messages[lang][ErrorCode(code)] = msg
	}

	return nil
}

// ============================================================================
// 消息获取函数
// ============================================================================

// GetMessage 获取指定语言的错误消息
// 如果找不到对应语言的消息，返回中文消息
// 如果中文消息也没有，返回错误码本身
func GetMessage(code ErrorCode, lang string) string {
	globalMessageManager.mu.RLock()
	defer globalMessageManager.mu.RUnlock()

	// 规范化语言代码
	lang = common.NormalizeLanguage(lang)

	// 尝试获取指定语言的消息
	if msgs, ok := globalMessageManager.messages[lang]; ok {
		if msg, ok := msgs[code]; ok {
			return msg
		}
	}

	// 回退到中文
	if msgs, ok := globalMessageManager.messages["zh-CN"]; ok {
		if msg, ok := msgs[code]; ok {
			return msg
		}
	}

	// 返回错误码本身
	return string(code)
}

// ============================================================================
// AppError 多语言支持
// ============================================================================

// GetMessage 获取指定语言的消息
func (e *AppError) GetMessage(lang string) string {
	return GetMessage(e.Code, lang)
}

// LocalizedResponse 本地化响应结构
type LocalizedResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"`
}

// ToLocalizedResponse 转换为本地化响应
func (e *AppError) ToLocalizedResponse(lang string) *LocalizedResponse {
	return &LocalizedResponse{
		Code:    e.Code,
		Message: e.GetMessage(lang),
		Details: e.Details,
	}
}
