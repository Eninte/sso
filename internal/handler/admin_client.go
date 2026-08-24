// Package handler 管理员客户端管理处理器
// 提供 OAuth 客户端 CRUD / 禁用启用 / 密钥轮换的管理员 API
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	apperrors "github.com/example/sso/internal/errors"
	"github.com/example/sso/internal/middleware"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/service"
)

// ============================================================================
// 客户端管理
// ============================================================================

// clientResponse 将客户端转换为响应对象
//
// 安全：绝不包含 client_secret 哈希（model.Client.ClientSecret 为 json:"-"，
// 此处手工构建同样不复制该字段）。secret 明文仅在创建/轮换时通过 clientSecret 参数单独返回一次。
func clientResponse(client *model.Client, clientSecret string) map[string]interface{} {
	resp := map[string]interface{}{
		"client_id":     client.ClientID,
		"name":          client.Name,
		"redirect_uris": client.RedirectURIs,
		"grant_types":   client.GrantTypes,
		"scopes":        client.Scopes,
		"public_client": client.PublicClient,
		"is_active":     client.IsActive,
		"created_at":    client.CreatedAt,
		"updated_at":    client.UpdatedAt,
	}
	if clientSecret != "" {
		resp["client_secret"] = clientSecret
	}
	return resp
}

// adminClientContext 组装客户端管理操作的请求上下文
func (h *AdminHandler) adminClientContext(r *http.Request) (context.Context, string) {
	operatorID := middleware.GetUserIDFromContext(r.Context())
	ctx := service.WithClientIP(r.Context(), extractClientIP(r))
	return ctx, operatorID
}

// HandleCreateClient 处理创建客户端请求
// POST /admin/clients
func (h *AdminHandler) HandleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req model.ClientCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	ctx, operatorID := h.adminClientContext(r)
	client, clientSecret, err := h.adminSvc.CreateClient(ctx, operatorID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, clientResponse(client, clientSecret))
}

// HandleListClients 处理客户端列表请求
// GET /admin/clients?page=1&pageSize=20
func (h *AdminHandler) HandleListClients(w http.ResponseWriter, r *http.Request) {
	page := 1
	pageSize := DefaultPageSize

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr := r.URL.Query().Get("pageSize"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= MaxPageSize {
			pageSize = ps
		}
	}

	offset := (page - 1) * pageSize

	clients, total, err := h.adminSvc.ListClients(r.Context(), offset, pageSize)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	clientList := make([]map[string]interface{}, 0, len(clients))
	for _, client := range clients {
		clientList = append(clientList, clientResponse(client, ""))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"clients":     clientList,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
}

// HandleGetClient 处理获取客户端请求
// GET /admin/clients/{client_id}
func (h *AdminHandler) HandleGetClient(w http.ResponseWriter, r *http.Request) {
	clientID := mux.Vars(r)["client_id"]
	if clientID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}

	client, err := h.adminSvc.GetClient(r.Context(), clientID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, clientResponse(client, ""))
}

// HandleUpdateClient 处理更新客户端请求
// PUT /admin/clients/{client_id}
func (h *AdminHandler) HandleUpdateClient(w http.ResponseWriter, r *http.Request) {
	clientID := mux.Vars(r)["client_id"]
	if clientID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}

	var req model.ClientUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		handleDecodeJSONError(w, r, err)
		return
	}

	ctx, operatorID := h.adminClientContext(r)
	client, err := h.adminSvc.UpdateClient(ctx, operatorID, clientID, req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, clientResponse(client, ""))
}

// HandleDisableClient 处理禁用客户端请求
// POST /admin/clients/{client_id}/disable
func (h *AdminHandler) HandleDisableClient(w http.ResponseWriter, r *http.Request) {
	clientID := mux.Vars(r)["client_id"]
	if clientID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}

	ctx, operatorID := h.adminClientContext(r)
	if err := h.adminSvc.DisableClient(ctx, operatorID, clientID); err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeSuccess(w, http.StatusOK, "客户端已禁用", nil)
}

// HandleEnableClient 处理启用客户端请求
// POST /admin/clients/{client_id}/enable
func (h *AdminHandler) HandleEnableClient(w http.ResponseWriter, r *http.Request) {
	clientID := mux.Vars(r)["client_id"]
	if clientID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}

	ctx, operatorID := h.adminClientContext(r)
	if err := h.adminSvc.EnableClient(ctx, operatorID, clientID); err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeSuccess(w, http.StatusOK, "客户端已启用", nil)
}

// HandleRotateClientSecret 处理轮换客户端密钥请求
// POST /admin/clients/{client_id}/rotate-secret
func (h *AdminHandler) HandleRotateClientSecret(w http.ResponseWriter, r *http.Request) {
	clientID := mux.Vars(r)["client_id"]
	if clientID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}

	ctx, operatorID := h.adminClientContext(r)
	clientSecret, err := h.adminSvc.RotateClientSecret(ctx, operatorID, clientID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"client_id":     clientID,
		"client_secret": clientSecret,
	})
}

// HandleDeleteClient 处理删除客户端请求
// DELETE /admin/clients/{client_id}
func (h *AdminHandler) HandleDeleteClient(w http.ResponseWriter, r *http.Request) {
	clientID := mux.Vars(r)["client_id"]
	if clientID == "" {
		writeError(w, http.StatusBadRequest, getMessage(r, apperrors.ErrCodeMissingClientID))
		return
	}

	ctx, operatorID := h.adminClientContext(r)
	if err := h.adminSvc.DeleteClient(ctx, operatorID, clientID); err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeSuccess(w, http.StatusOK, "客户端已删除", nil)
}
