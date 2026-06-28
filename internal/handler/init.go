package handler

import (
	_ "embed"
	"net"
	"net/http"
	"text/template"

	"github.com/example/sso/internal/cache"
	"github.com/example/sso/internal/crypto"
	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/service"
	"github.com/example/sso/internal/store"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/handlerutil"
	"github.com/example/sso/internal/util/serviceutil"
)

//go:embed templates/init.html
var initHTMLTemplate string

var initTmpl = template.Must(template.New("init").Parse(initHTMLTemplate))

// InitHandler 初始化面板handler
// 服务正常运行后使用，提供系统状态查看、管理员创建、OAuth客户端创建
type InitHandler struct {
	initSvc   service.InitServiceInterface
	cache     cache.Cache
	auditSvc  auditutil.AuditService
	store     store.Store
	version   string
	buildTime string
}

// NewInitHandler 创建初始化面板handler
func NewInitHandler(store store.Store, passwordSvc *crypto.PasswordService, cache cache.Cache, auditSvc auditutil.AuditService, version, buildTime string) *InitHandler {
	return &InitHandler{
		initSvc:   service.NewInitService(store, passwordSvc, auditSvc),
		cache:     cache,
		auditSvc:  auditSvc,
		store:     store,
		version:   version,
		buildTime: buildTime,
	}
}

// HandleInitPage 渲染初始化面板页面
// 安全限制：仅允许本地访问（127.0.0.1, ::1, localhost）
func (h *InitHandler) HandleInitPage(w http.ResponseWriter, r *http.Request) {
	// 检查是否为本地访问
	if !isLocalRequest(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("初始化面板仅允许本地访问"))
		return
	}

	exists, err := h.initSvc.AdminExists(r.Context())
	if err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("检查管理员状态失败"))
		return
	}
	if exists {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("初始化已完成"))
		return
	}

	nonce := middleware.GetCSPNonce(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = initTmpl.Execute(w, map[string]string{
		"Nonce":     nonce,
		"Version":   h.version,
		"BuildTime": h.buildTime,
	})
}

// HandleSystemStatus 返回系统状态JSON
// 安全限制：仅允许本地访问（127.0.0.1, ::1, localhost）
func (h *InitHandler) HandleSystemStatus(w http.ResponseWriter, r *http.Request) {
	// 检查是否为本地访问
	if !isLocalRequest(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("初始化面板仅允许本地访问"))
		return
	}

	exists, err := h.initSvc.AdminExists(r.Context())
	if err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrInternal.WithDetails("检查管理员状态失败"))
		return
	}
	if exists {
		handlerutil.WriteJSONError(w, apperrors.ErrNotFound)
		return
	}

	status := map[string]interface{}{}

	if err := h.store.Ping(r.Context()); err != nil {
		status["db"] = map[string]string{"status": "error", "message": "数据库连接异常"}
	} else {
		status["db"] = map[string]string{"status": "ok"}
	}

	if rc, ok := h.cache.(*cache.RedisCache); ok {
		if err := rc.Ping(r.Context()); err != nil {
			status["redis"] = map[string]string{"status": "error", "message": "Redis连接异常"}
		} else {
			status["redis"] = map[string]string{"status": "ok"}
		}
	} else {
		status["redis"] = map[string]string{"status": "disabled", "message": "使用内存缓存"}
	}

	status["version"] = h.version
	status["build_time"] = h.buildTime

	handlerutil.WriteJSONSuccess(w, status)
}

// HandleCreateAdmin 创建管理员账户
// 安全限制：仅允许本地访问（127.0.0.1, ::1, localhost）
func (h *InitHandler) HandleCreateAdmin(w http.ResponseWriter, r *http.Request) {
	// 检查是否为本地访问
	if !isLocalRequest(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("初始化面板仅允许本地访问"))
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}

	user, err := h.initSvc.CreateAdmin(r.Context(), req.Email, req.Password)
	if err != nil {
		if apperrors.Is(err, apperrors.ErrForbidden) {
			handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("管理员已存在，不允许重复创建"))
			return
		}
		if apperrors.Is(err, apperrors.ErrEmailExists) {
			handlerutil.WriteJSONError(w, apperrors.ErrEmailExists)
			return
		}
		handlerutil.WriteJSONError(w, serviceutil.HandleStoreError(err, apperrors.ErrInternal))
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]string{
		"id":    user.ID,
		"email": user.Email,
	})
}

// HandleCreateClient 创建默认OAuth客户端
// 安全限制：仅允许本地访问（127.0.0.1, ::1, localhost）
func (h *InitHandler) HandleCreateClient(w http.ResponseWriter, r *http.Request) {
	// 检查是否为本地访问
	if !isLocalRequest(r) {
		handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("初始化面板仅允许本地访问"))
		return
	}

	var req struct {
		Name        string `json:"name"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := decodeJSON(r, &req); err != nil {
		handlerutil.WriteJSONError(w, apperrors.ErrBadRequest.WithDetails("无效的请求格式"))
		return
	}

	client, clientSecret, err := h.initSvc.CreateOAuthClient(r.Context(), req.Name, req.RedirectURI)
	if err != nil {
		if apperrors.Is(err, apperrors.ErrForbidden) {
			handlerutil.WriteJSONError(w, apperrors.ErrForbidden.WithDetails("请先创建管理员账户"))
			return
		}
		handlerutil.WriteJSONError(w, serviceutil.HandleStoreError(err, apperrors.ErrInternal))
		return
	}

	handlerutil.WriteJSONSuccess(w, map[string]string{
		"client_id":     client.ClientID,
		"client_secret": clientSecret,
	})
}

// isLocalRequest 检查请求是否来自本地
// 允许的本地地址：127.0.0.1, ::1, localhost
func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// SplitHostPort 失败时（如没有端口），直接使用 RemoteAddr
		host = r.RemoteAddr
	}

	localAddresses := []string{
		"127.0.0.1",
		"::1",
		"localhost",
	}

	for _, local := range localAddresses {
		if host == local {
			return true
		}
	}

	return false
}
