use sso_sdk::{SSOClient, SSOError, ErrorCode};

// ============================================================================
// 基础测试
// ============================================================================

#[tokio::test]
async fn test_create_client() {
    let client = SSOClient::new("http://localhost:9090");
    assert!(client.access_token().await.is_none());
}

#[tokio::test]
async fn test_trailing_slash() {
    // 验证不会 panic
    let _ = SSOClient::new("http://localhost:9090/");
}

#[tokio::test]
async fn test_set_clear_tokens() {
    let client = SSOClient::new("http://localhost:9090");
    client.set_tokens("a", "b", 900).await;
    assert_eq!(client.access_token().await, Some("a".to_string()));

    client.clear_tokens().await;
    assert!(client.access_token().await.is_none());
}

#[tokio::test]
async fn test_user_info_no_token() {
    let client = SSOClient::new("http://localhost:9090");
    let result = client.user_info().await;
    assert!(result.is_err());
    let err = result.unwrap_err();
    assert!(err.is_unauthorized());
}

// ============================================================================
// Error 测试
// ============================================================================

#[test]
fn test_error_methods() {
    let err = SSOError {
        http_status: 404,
        code: ErrorCode::NotFound,
        message: "not found".to_string(),
        raw_body: "{}".to_string(),
    };
    assert!(err.is_not_found());
    assert!(!err.is_unauthorized());
    assert!(!err.is_forbidden());
    assert!(!err.is_conflict());
    assert!(!err.is_rate_limited());
}

#[test]
fn test_error_codes() {
    assert_eq!(ErrorCode::from_str("NOT_FOUND"), ErrorCode::NotFound);
    assert_eq!(ErrorCode::from_str("UNAUTHORIZED"), ErrorCode::Unauthorized);
    assert_eq!(ErrorCode::from_str("CONFLICT"), ErrorCode::Conflict);
    assert_eq!(
        ErrorCode::from_str("CUSTOM_ERROR"),
        ErrorCode::Other("CUSTOM_ERROR".to_string())
    );
}

#[test]
fn test_error_display() {
    assert_eq!(ErrorCode::NotFound.to_string(), "NOT_FOUND");
    assert_eq!(ErrorCode::Unauthorized.to_string(), "UNAUTHORIZED");
    assert_eq!(ErrorCode::EmailExists.to_string(), "EMAIL_EXISTS");
}

// ============================================================================
// mockito 集成测试（需要 mockito crate）
// ============================================================================

#[cfg(test)]
mod mock_tests {
    use super::*;
    use mockito::Server;

    #[tokio::test]
    async fn test_register() {
        let mut server = Server::new_async().await;
        let mock = server
            .mock("POST", "/api/v1/register")
            .with_status(201)
            .with_header("content-type", "application/json")
            .with_body(r#"{"message":"注册成功","data":{"user_id":"u1","email":"t@e.com"}}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        let resp = client.register("t@e.com", "P@ss1").await.unwrap();

        assert_eq!(resp.message, "注册成功");
        assert_eq!(resp.data.unwrap().user_id, "u1");
        mock.assert_async().await;
    }

    #[tokio::test]
    async fn test_register_conflict() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/register")
            .with_status(409)
            .with_header("content-type", "application/json")
            .with_body(r#"{"code":"EMAIL_EXISTS","message":"exists"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        let err = client.register("x@e.com", "P@ss1").await.unwrap_err();

        assert!(err.is_conflict());
        assert_eq!(err.code, ErrorCode::EmailExists);
    }

    #[tokio::test]
    async fn test_login() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/login")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(
                r#"{"access_token":"a1","refresh_token":"r1","token_type":"Bearer","expires_in":900}"#,
            )
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        let resp = client.login("t@e.com", "P@ss1").await.unwrap();

        assert_eq!(resp.access_token, "a1");
        assert_eq!(client.access_token().await, Some("a1".to_string()));
    }

    #[tokio::test]
    async fn test_login_fail() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/login")
            .with_status(401)
            .with_header("content-type", "application/json")
            .with_body(r#"{"code":"INVALID_CREDENTIALS","message":"wrong"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        let err = client.login("t@e.com", "wrong").await.unwrap_err();

        assert!(err.is_unauthorized());
    }

    #[tokio::test]
    async fn test_user_info() {
        let mut server = Server::new_async().await;
        server
            .mock("GET", "/api/v1/userinfo")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"sub":"u1","email":"t@e.com","email_verified":true}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let info = client.user_info().await.unwrap();

        assert_eq!(info.sub, "u1");
        assert_eq!(info.email, "t@e.com");
    }

    #[tokio::test]
    async fn test_revoke() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/token/revoke")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"message":"ok"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        client.revoke_token().await.unwrap();

        assert!(client.access_token().await.is_none());
    }

    #[tokio::test]
    async fn test_mfa_setup() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/mfa/setup")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"secret":"ABC","qr_code_url":"otpauth://","manual_entry":"ABC"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let resp = client.mfa_setup().await.unwrap();

        assert_eq!(resp.secret, "ABC");
    }

    #[tokio::test]
    async fn test_admin_health() {
        let mut server = Server::new_async().await;
        server
            .mock("GET", "/api/v1/admin/health")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"status":"ok","timestamp":"2026-01-01T00:00:00Z","database":"pg","version":"1.0"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let resp = client.admin_health().await.unwrap();

        assert_eq!(resp.status, "ok");
    }

    #[tokio::test]
    async fn test_admin_cleanup() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/admin/cleanup")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"message":"cleanup done"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let resp = client.admin_cleanup().await.unwrap();

        assert_eq!(resp.message, "cleanup done");
    }

    #[tokio::test]
    async fn test_get_user() {
        let mut server = Server::new_async().await;
        server
            .mock("GET", "/api/v1/admin/users/u1")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"id":"u1","email":"a@b.com","email_verified":true,"mfa_enabled":false,"status":"active","created_at":"","updated_at":""}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let resp = client.get_user("u1").await.unwrap();

        assert_eq!(resp.id, "u1");
        assert_eq!(resp.email, "a@b.com");
    }

    #[tokio::test]
    async fn test_disable_user() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/admin/users/u1/disable")
            .match_body(mockito::Matcher::Missing)
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"message":"user disabled"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let resp = client.disable_user("u1").await.unwrap();

        assert_eq!(resp.message, "user disabled");
    }

    #[tokio::test]
    async fn test_enable_user() {
        let mut server = Server::new_async().await;
        server
            .mock("POST", "/api/v1/admin/users/u1/enable")
            .match_body(mockito::Matcher::Missing)
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"message":"user enabled"}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let resp = client.enable_user("u1").await.unwrap();

        assert_eq!(resp.message, "user enabled");
    }

    #[tokio::test]
    async fn test_discovery() {
        let mut server = Server::new_async().await;
        server
            .mock("GET", "/.well-known/openid-configuration")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(
                r#"{"issuer":"http://t","authorization_endpoint":"http://t/authorize","token_endpoint":"http://t/token","userinfo_endpoint":"http://t/userinfo","jwks_uri":"http://t/jwks","revocation_endpoint":"http://t/revoke","grant_types_supported":["authorization_code"],"code_challenge_methods_supported":["S256"]}"#,
            )
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        let resp = client.discovery().await.unwrap();

        assert_eq!(resp.issuer, "http://t");
        assert!(resp.grant_types_supported.contains(&"authorization_code".to_string()));
    }

    #[tokio::test]
    async fn test_jwks() {
        let mut server = Server::new_async().await;
        server
            .mock("GET", "/.well-known/jwks.json")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"keys":[{"kty":"RSA","use":"sig","kid":"k1","n":"a","e":"b"}]}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        let resp = client.jwks().await.unwrap();

        assert_eq!(resp.keys.len(), 1);
        assert_eq!(resp.keys[0].kty, "RSA");
    }

    #[tokio::test]
    async fn test_list_users() {
        let mut server = Server::new_async().await;
        server
            .mock("GET", "/api/v1/admin/users?page=1&pageSize=10")
            .with_status(200)
            .with_header("content-type", "application/json")
            .with_body(r#"{"users":[{"id":"u1","email":"a@b.com","email_verified":true,"mfa_enabled":false,"status":"active","created_at":"","updated_at":""}],"total":1,"page":1,"page_size":10,"total_pages":1}"#)
            .create_async()
            .await;

        let client = SSOClient::new(&server.url());
        client.set_tokens("tok", "r", 900).await;
        let resp = client.list_users(1, 10).await.unwrap();

        assert_eq!(resp.total, 1);
        assert_eq!(resp.users[0].id, "u1");
    }
}
