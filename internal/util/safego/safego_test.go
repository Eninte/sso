package safego

import (
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func TestGo_NormalExecution(t *testing.T) {
	var executed atomic.Bool
	Go(nil, "test", func() {
		executed.Store(true)
	})

	// 等待 goroutine 执行完成
	waitFor(t, func() bool { return executed.Load() })

	if !executed.Load() {
		t.Fatal("goroutine 未执行")
	}
}

func TestGo_PanicRecovery(t *testing.T) {
	var executed atomic.Bool
	Go(nil, "panic-test", func() {
		executed.Store(true)
		panic("test panic")
	})

	// 等待 goroutine 执行（应在 panic 前设置 flag）
	waitFor(t, func() bool { return executed.Load() })

	if !executed.Load() {
		t.Fatal("goroutine 未执行到 panic 点")
	}
}

func TestGo_PanicRecoveryWithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var executed atomic.Bool
	Go(logger, "panic-with-logger", func() {
		executed.Store(true)
		panic("test panic with logger")
	})

	waitFor(t, func() bool { return executed.Load() })

	if !executed.Load() {
		t.Fatal("goroutine 未执行到 panic 点")
	}
}

// waitFor 轮询等待条件满足，最多等 1 秒
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
