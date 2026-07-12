// Package safego 提供安全的 goroutine 启动工具
// 自动 recover panic，防止 goroutine panic 导致进程崩溃
package safego

import "log/slog"

// Go 启动一个安全的 goroutine，自动 recover panic
// logger: 日志记录器，panic 时记录错误日志
// msg: 描述信息，用于日志标识
// fn: 要执行的函数
func Go(logger *slog.Logger, msg string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.Error("goroutine panic", "message", msg, "panic", r)
				}
			}
		}()
		fn()
	}()
}
