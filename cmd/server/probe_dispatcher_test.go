package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProbeDispatcher_RoutesProbesToProbeHandler(t *testing.T) {
	var probeHits int32
	var mainHits int32

	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&probeHits, 1)
		w.WriteHeader(http.StatusOK)
	})
	main := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mainHits, 1)
		w.WriteHeader(http.StatusOK)
	})

	disp := probeDispatcher(probe, main)

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		disp.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}

	// 业务路径走主路由
	req := httptest.NewRequest("GET", "/api/v1/login", nil)
	rec := httptest.NewRecorder()
	disp.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, int32(2), probeHits, "探针路径应全部命中探针路由器")
	assert.Equal(t, int32(1), mainHits, "业务路径应命中主路由")
}

func TestProbeDispatcher_ProbeBypassesMainMiddleware(t *testing.T) {
	// 模拟主路由的限流中间件：拦截所有请求返回429
	mainWithRateLimit := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	probe := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	disp := probeDispatcher(probe, mainWithRateLimit)

	// 探针不应被限流
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	disp.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "探针必须绕过主路由限流")

	// 业务路径仍走主路由（被限流）
	req = httptest.NewRequest("GET", "/api/v1/login", nil)
	rec = httptest.NewRecorder()
	disp.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "业务路径仍受主路由限流")
}
