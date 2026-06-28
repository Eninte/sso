//go:build integration
// +build integration

package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/sso/internal/crypto"
	"github.com/example/sso/internal/handler"
	"github.com/example/sso/internal/model"
	"github.com/example/sso/internal/store/postgres"
	"github.com/example/sso/internal/util/auditutil"
	"github.com/example/sso/internal/util/testutil"
)

// mockAuditService 模拟审计服务
type mockAuditService struct {
	logs []*model.AuditLog
}

func (m *mockAuditService) Log(ctx context.Context, log *model.AuditLog) {
	m.logs = append(m.logs, log)
}

var _ auditutil.AuditService = (*mockAuditService)(nil)

// setupTestDB 返回已 ping 通的真实 PG 连接（带重试与超时）
// 复用 testutil.ConnectTestDB，与全仓真实 DB 测试共享重试机制
func setupTestDB(t *testing.T) (*sql.DB, func()) {
	db := testutil.ConnectTestDB(t)

	// 清理测试数据
	cleanup := func() {
		_, _ = db.Exec("DELETE FROM users WHERE email LIKE 'test-init-%'")
		_, _ = db.Exec("DELETE FROM oauth_clients WHERE name LIKE 'Test Init Client%'")
		db.Close()
	}

	return db, cleanup
}

func TestInitHandler_HandleInitPage_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// 确保没有管理员
	_, _ = db.Exec("DELETE FROM users WHERE role = 'admin'")

	store := postgres.New(db)

	auditSvc := &mockAuditService{}
	h := handler.NewInitHandler(store, nil, nil, auditSvc, "1.0.0", "2024-01-01")

	req := httptest.NewRequest("GET", "/init", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	h.HandleInitPage(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "SSO 部署初始化")
}

func TestInitHandler_HandleSystemStatus_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// 确保没有管理员
	_, _ = db.Exec("DELETE FROM users WHERE role = 'admin'")

	store := postgres.New(db)

	auditSvc := &mockAuditService{}
	h := handler.NewInitHandler(store, nil, nil, auditSvc, "1.0.0", "2024-01-01")

	req := httptest.NewRequest("GET", "/api/v1/init/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	h.HandleSystemStatus(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Contains(t, data, "db")
	assert.Contains(t, data, "version")
	assert.Equal(t, "1.0.0", data["version"])
}

func TestInitHandler_HandleCreateAdmin_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// 确保没有管理员
	_, _ = db.Exec("DELETE FROM users WHERE role = 'admin'")

	store := postgres.New(db)

	passwordSvc := crypto.NewPasswordService(4) // 使用低成本加快测试
	auditSvc := &mockAuditService{}
	h := handler.NewInitHandler(store, passwordSvc, nil, auditSvc, "1.0.0", "2024-01-01")

	requestBody := map[string]string{
		"email":    "test-init-admin@example.com",
		"password": "AdminPassword123!",
	}

	var body bytes.Buffer
	json.NewEncoder(&body).Encode(requestBody)

	req := httptest.NewRequest("POST", "/api/v1/init/admin", &body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleCreateAdmin(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.Equal(t, "test-init-admin@example.com", data["email"])
	assert.NotEmpty(t, data["id"])

	// 验证审计日志
	assert.Len(t, auditSvc.logs, 1)
	assert.Equal(t, "admin_created", auditSvc.logs[0].EventType)

	// 验证管理员已创建
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE email = $1 AND role = 'admin'", "test-init-admin@example.com").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestInitHandler_HandleCreateAdmin_DuplicateEmail_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// 确保没有管理员
	_, _ = db.Exec("DELETE FROM users WHERE role = 'admin'")

	store := postgres.New(db)

	passwordSvc := crypto.NewPasswordService(4)
	auditSvc := &mockAuditService{}

	// 先创建一个用户
	now := time.Now()
	user := &model.User{
		ID:            "550e8400-e29b-41d4-a716-446655440000", // 使用有效的UUID
		Email:         "test-init-duplicate@example.com",
		PasswordHash:  "hash",
		EmailVerified: true,
		Role:          model.UserRoleUser,
		Status:        model.UserStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	err := store.Create(context.Background(), user)
	require.NoError(t, err)

	h := handler.NewInitHandler(store, passwordSvc, nil, auditSvc, "1.0.0", "2024-01-01")

	requestBody := map[string]string{
		"email":    "test-init-duplicate@example.com",
		"password": "AdminPassword123!",
	}

	var body bytes.Buffer
	json.NewEncoder(&body).Encode(requestBody)

	req := httptest.NewRequest("POST", "/api/v1/init/admin", &body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleCreateAdmin(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "EMAIL_EXISTS")
}

func TestInitHandler_HandleCreateClient_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// 确保没有管理员
	_, _ = db.Exec("DELETE FROM users WHERE role = 'admin'")

	store := postgres.New(db)

	passwordSvc := crypto.NewPasswordService(4)
	auditSvc := &mockAuditService{}
	h := handler.NewInitHandler(store, passwordSvc, nil, auditSvc, "1.0.0", "2024-01-01")

	// 先创建管理员（创建客户端需要先有管理员）
	createAdminBody := map[string]string{
		"email":    "test-init-admin-for-client@example.com",
		"password": "AdminPassword123!",
	}
	var adminBody bytes.Buffer
	json.NewEncoder(&adminBody).Encode(createAdminBody)
	adminReq := httptest.NewRequest("POST", "/api/v1/init/admin", &adminBody)
	adminReq.RemoteAddr = "127.0.0.1:12345"
	adminReq.Header.Set("Content-Type", "application/json")
	adminW := httptest.NewRecorder()
	h.HandleCreateAdmin(adminW, adminReq)
	if adminW.Code != http.StatusOK {
		t.Fatalf("Failed to create admin: %d, body: %s", adminW.Code, adminW.Body.String())
	}

	// 现在创建客户端
	requestBody := map[string]string{
		"name":         "Test Init Client",
		"redirect_uri": "http://localhost:3000/callback",
	}

	var body bytes.Buffer
	json.NewEncoder(&body).Encode(requestBody)

	req := httptest.NewRequest("POST", "/api/v1/init/client", &body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleCreateClient(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	data := response["data"].(map[string]interface{})
	assert.NotEmpty(t, data["client_id"])
	assert.NotEmpty(t, data["client_secret"])

	// 验证审计日志（先有admin_created，再有oauth_client_created）
	assert.Len(t, auditSvc.logs, 2)
	assert.Equal(t, "oauth_client_created", auditSvc.logs[1].EventType)

	// 验证客户端已创建（使用正确的表名 oauth_clients）
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM oauth_clients WHERE name = $1", "Test Init Client").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestInitHandler_AdminExists_ReturnsNotFound_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	store := postgres.New(db)

	// 创建一个管理员
	now := time.Now()
	admin := &model.User{
		ID:            "660e8400-e29b-41d4-a716-446655440000", // 使用有效的UUID
		Email:         "test-init-admin-exists@example.com",
		PasswordHash:  "hash",
		EmailVerified: true,
		Role:          model.UserRoleAdmin,
		Status:        model.UserStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	err := store.Create(context.Background(), admin)
	require.NoError(t, err)

	auditSvc := &mockAuditService{}
	passwordSvc := crypto.NewPasswordService(4)
	h := handler.NewInitHandler(store, passwordSvc, nil, auditSvc, "1.0.0", "2024-01-01")

	// 测试 HandleInitPage
	req := httptest.NewRequest("GET", "/init", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	h.HandleInitPage(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 测试 HandleSystemStatus
	req = httptest.NewRequest("GET", "/api/v1/init/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	h.HandleSystemStatus(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 测试 HandleCreateAdmin
	requestBody := map[string]string{
		"email":    "another-admin@example.com",
		"password": "AdminPassword123!",
	}
	var body bytes.Buffer
	json.NewEncoder(&body).Encode(requestBody)
	req = httptest.NewRequest("POST", "/api/v1/init/admin", &body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.HandleCreateAdmin(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 测试 HandleCreateClient
	requestBody = map[string]string{
		"name":         "Test Client",
		"redirect_uri": "http://localhost:3000/callback",
	}
	body.Reset()
	json.NewEncoder(&body).Encode(requestBody)
	req = httptest.NewRequest("POST", "/api/v1/init/client", &body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.HandleCreateClient(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 验证客户端创建成功
	var clientResp map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&clientResp)
	require.NoError(t, err)
	clientData := clientResp["data"].(map[string]interface{})
	assert.NotEmpty(t, clientData["client_id"])
	assert.NotEmpty(t, clientData["client_secret"])
}
