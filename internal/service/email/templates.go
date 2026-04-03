// Package email 邮件服务
package email

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"sync"
)

//go:embed templates/*.html
var templateFS embed.FS

// TemplateManager 邮件模板管理器
type TemplateManager struct {
	mu        sync.RWMutex
	templates map[string]*template.Template
}

// NewTemplateManager 创建模板管理器
func NewTemplateManager() (*TemplateManager, error) {
	tm := &TemplateManager{
		templates: make(map[string]*template.Template),
	}

	// 加载所有模板
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("读取模板目录失败: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		tmpl, err := template.ParseFS(templateFS, "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("解析模板 %s: %w", name, err)
		}

		tm.templates[name] = tmpl
	}

	return tm, nil
}

// Render 渲染模板
func (tm *TemplateManager) Render(name string, data interface{}) (string, error) {
	tm.mu.RLock()
	tmpl, ok := tm.templates[name]
	tm.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("模板不存在: %s", name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("渲染模板 %s: %w", name, err)
	}

	return buf.String(), nil
}
