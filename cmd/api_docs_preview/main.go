// API 文档页面本地预览程序（仅用于开发调试，不参与生产构建）
// 用法: go run ./cmd/api_docs_preview
// 访问: http://127.0.0.1:8080/api-docs
//
// 该程序直接复用 internal/handler.APIDocsHandler，保证渲染逻辑与生产一致。
// 注意：本预览程序**不带鉴权**，仅供本地预览，禁止用于生产。
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/middleware"
)

const listenAddr = "127.0.0.1:8080"

func main() {
	// 复用生产 handler，保证渲染逻辑完全一致
	h := handler.NewAPIDocsHandler("http://127.0.0.1:8080", "preview-local")

	mux := http.NewServeMux()

	// Scalar 页面（用 SecurityHeaders 中间件注入 CSP nonce 到 context）
	// 同时注册 /api-docs（预览路径）和 /api/v1/admin/api-docs（生产路径）
	// 因为 HTML 模板里写死了生产路径，预览程序需要同时支持
	mux.Handle("/api-docs", middleware.SecurityHeaders(http.HandlerFunc(h.HandlePage)))
	mux.Handle("/api/v1/admin/api-docs", middleware.SecurityHeaders(http.HandlerFunc(h.HandlePage)))

	// OpenAPI 规范
	mux.HandleFunc("/api-docs/openapi.json", h.HandleSpec)
	mux.HandleFunc("/api/v1/admin/api-docs/openapi.json", h.HandleSpec)

	// Scalar JS
	mux.HandleFunc("/api-docs/scalar.js", h.HandleScalarJS)
	mux.HandleFunc("/api/v1/admin/api-docs/scalar.js", h.HandleScalarJS)

	// 根路径重定向
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/api-docs", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	fmt.Printf("API 文档预览服务已启动\n")
	fmt.Printf("访问:     http://%s/api-docs\n", listenAddr)
	fmt.Printf("OpenAPI:  http://%s/api-docs/openapi.json\n", listenAddr)
	fmt.Printf("Scalar JS: http://%s/api-docs/scalar.js\n", listenAddr)
	fmt.Printf("\n按 Ctrl+C 退出\n\n")

	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
