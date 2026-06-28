// Package app 配置向导服务
// 当 config.Load() 失败时启动轻量 HTTP 服务，引导用户完成首次配置
package app

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"

	"github.com/example/sso/internal/config"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/middleware"
)

// startSetupWizard 启动配置向导HTTP服务
// 当config.Load()失败时调用，启动轻量HTTP服务显示配置向导
// 阻塞直到收到配置写入后通过syscall.Exec重启进程
func startSetupWizard(loadErr error, version string) {
	envPath := config.GetEnvPath()
	setupHandler := handler.NewSetupHandler(envPath, version)

	router := mux.NewRouter()
	router.Use(middleware.SecurityHeaders)
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)

	// 配置向导限流：10请求/分钟，防止暴力攻击
	setupRateLimiter := middleware.NewRateLimiter(10, time.Minute)
	defer setupRateLimiter.Stop()
	router.Use(setupRateLimiter.Middleware)

	router.HandleFunc("/setup", setupHandler.HandleSetupPage).Methods("GET")
	router.HandleFunc("/api/v1/setup/save", setupHandler.HandleSetupSave).Methods("POST")
	router.HandleFunc("/api/v1/setup/test-db", setupHandler.HandleSetupTestDB).Methods("POST")
	router.HandleFunc("/api/v1/setup/test-redis", setupHandler.HandleSetupTestRedis).Methods("POST")
	router.HandleFunc("/api/v1/setup/generate-keys", setupHandler.HandleSetupGenerateKeys).Methods("POST")

	// 根路径重定向到/setup
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/setup", http.StatusFound)
	})

	addr := os.Getenv("SERVER_HOST") + ":" + os.Getenv("SERVER_PORT")
	if addr == ":" {
		addr = "127.0.0.1:9090"
	}
	// nosec G706 -- slog 结构化日志，值作为参数传递不受注入影响
	slog.Info("配置向导启动", "address", addr, "config_error", loadErr.Error())

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		slog.Error("配置向导服务启动失败", "error", err)
		os.Exit(1)
	}
}
