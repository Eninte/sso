use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};

use reqwest::{Client as HttpClient, Method};

use crate::errors::{parse_error, SSOError};
use crate::models::*;

/// Token 内部状态
struct TokenState {
    access_token: String,
    refresh_token: String,
    token_expiry: Instant,
}

/// SSO 客户端
pub struct SSOClient {
    base_url: String,
    http: HttpClient,
    tokens: Arc<RwLock<Option<TokenState>>>,
}

impl SSOClient {
    /// 创建新的 SSO 客户端
    pub fn new(base_url: impl Into<String>) -> Self {
        let base_url = base_url.into().trim_end_matches('/').to_string();
        Self {
            base_url,
            http: HttpClient::builder()
                .timeout(Duration::from_secs(30))
                .build()
                .expect("failed to build HTTP client"),
            tokens: Arc::new(RwLock::new(None)),
        }
    }

    /// 设置自定义 HTTP 客户端
    pub fn with_http_client(mut self, http: HttpClient) -> Self {
        self.http = http;
        self
    }

    /// 设置预设 Token
    pub fn with_tokens(
        self,
        access_token: impl Into<String>,
        refresh_token: impl Into<String>,
        expires_in: u64,
    ) -> Self {
        let state = TokenState {
            access_token: access_token.into(),
            refresh_token: refresh_token.into(),
            token_expiry: Instant::now() + Duration::from_secs(expires_in),
        };
        *self.tokens.write().expect("token lock poisoned") = Some(state);
        self
    }

    /// 获取当前 Access Token
    pub async fn access_token(&self) -> Option<String> {
        self.tokens.read().expect("token lock poisoned").as_ref().map(|t| t.access_token.clone())
    }

    /// 设置 Token
    pub async fn set_tokens(&self, access_token: &str, refresh_token: &str, expires_in: u64) {
        let mut guard = self.tokens.write().expect("token lock poisoned");
        *guard = Some(TokenState {
            access_token: access_token.to_string(),
            refresh_token: refresh_token.to_string(),
            token_expiry: Instant::now() + Duration::from_secs(expires_in),
        });
    }

    /// 清除 Token
    pub async fn clear_tokens(&self) {
        self.tokens.write().expect("token lock poisoned").take();
    }

    // =======================================================================
    // HTTP 请求
    // =======================================================================

    async fn request<T: serde::de::DeserializeOwned>(
        &self,
        method: Method,
        path: &str,
        body: Option<&impl serde::Serialize>,
        auth: bool,
    ) -> Result<T, SSOError> {
        let url = format!("{}{}", self.base_url, path);

        let mut req = self.http.request(method, &url);

        if auth {
            let token = self.ensure_token().await?;
            req = req.bearer_auth(&token);
        }

        if let Some(b) = body {
            req = req.json(b);
        }

        let resp = req.send().await.map_err(|e| SSOError {
            http_status: 0,
            code: crate::errors::ErrorCode::Other("REQUEST_FAILED".to_string()),
            message: e.to_string(),
            raw_body: String::new(),
        })?;

        let status = resp.status().as_u16();
        let text = resp.text().await.unwrap_or_default();

        if status >= 400 {
            return Err(parse_error(status, &text));
        }

        if text.is_empty() {
            // 返回空的默认值（用于 MessageResponse 等）
            return serde_json::from_str("{}").map_err(|e| SSOError {
                http_status: status,
                code: crate::errors::ErrorCode::Internal,
                message: e.to_string(),
                raw_body: text,
            });
        }

        serde_json::from_str(&text).map_err(|e| SSOError {
            http_status: status,
            code: crate::errors::ErrorCode::Internal,
            message: format!("parse response: {e}"),
            raw_body: text,
        })
    }

    async fn ensure_token(&self) -> Result<String, SSOError> {
        {
            let guard = self.tokens.read().expect("token lock poisoned");
            if let Some(ref state) = *guard {
                let needs_refresh = Instant::now() > state.token_expiry - Duration::from_secs(30);
                if !needs_refresh || state.refresh_token.is_empty() {
                    return Ok(state.access_token.clone());
                }
            } else {
                return Err(SSOError {
                    http_status: 401,
                    code: crate::errors::ErrorCode::Unauthorized,
                    message: "no access token available, please login first".to_string(),
                    raw_body: String::new(),
                });
            }
        }

        let refresh_token = {
            let guard = self.tokens.read().expect("token lock poisoned");
            guard.as_ref().unwrap().refresh_token.clone()
        };

        let resp: TokenResponse = self
            .request(
                Method::POST,
                "/api/v1/token",
                Some(&TokenRequest {
                    grant_type: "refresh_token".to_string(),
                    refresh_token: Some(refresh_token),
                    code: None,
                    redirect_uri: None,
                    client_id: None,
                    client_secret: None,
                    code_verifier: None,
                }),
                false,
            )
            .await?;

        self.set_tokens(&resp.access_token, &resp.refresh_token, resp.expires_in)
            .await;
        Ok(resp.access_token)
    }

    // =======================================================================
    // 认证
    // =======================================================================

    pub async fn register(
        &self,
        email: &str,
        password: &str,
    ) -> Result<RegisterResponse, SSOError> {
        self.request(
            Method::POST,
            "/api/v1/register",
            Some(&RegisterRequest {
                email: email.to_string(),
                password: password.to_string(),
            }),
            false,
        )
        .await
    }

    pub async fn login(
        &self,
        email: &str,
        password: &str,
    ) -> Result<TokenResponse, SSOError> {
        let resp: TokenResponse = self
            .request(
                Method::POST,
                "/api/v1/login",
                Some(&LoginRequest {
                    email: email.to_string(),
                    password: password.to_string(),
                }),
                false,
            )
            .await?;

        self.set_tokens(&resp.access_token, &resp.refresh_token, resp.expires_in)
            .await;
        Ok(resp)
    }

    pub async fn refresh_token(&self) -> Result<TokenResponse, SSOError> {
        let refresh_token = {
            let guard = self.tokens.read().expect("token lock poisoned");
            guard
                .as_ref()
                .map(|t| t.refresh_token.clone())
                .filter(|t| !t.is_empty())
                .ok_or_else(|| SSOError {
                    http_status: 401,
                    code: crate::errors::ErrorCode::Unauthorized,
                    message: "no refresh token available".to_string(),
                    raw_body: String::new(),
                })?
        };

        let resp: TokenResponse = self
            .request(
                Method::POST,
                "/api/v1/token",
                Some(&TokenRequest {
                    grant_type: "refresh_token".to_string(),
                    refresh_token: Some(refresh_token),
                    code: None,
                    redirect_uri: None,
                    client_id: None,
                    client_secret: None,
                    code_verifier: None,
                }),
                false,
            )
            .await?;

        self.set_tokens(&resp.access_token, &resp.refresh_token, resp.expires_in)
            .await;
        Ok(resp)
    }

    pub async fn exchange_code(
        &self,
        code: &str,
        client_id: &str,
        client_secret: &str,
        redirect_uri: &str,
        code_verifier: Option<&str>,
    ) -> Result<TokenResponse, SSOError> {
        self.request(
            Method::POST,
            "/api/v1/token",
            Some(&TokenRequest {
                grant_type: "authorization_code".to_string(),
                code: Some(code.to_string()),
                client_id: Some(client_id.to_string()),
                client_secret: Some(client_secret.to_string()),
                redirect_uri: Some(redirect_uri.to_string()),
                code_verifier: code_verifier.map(|s| s.to_string()),
                refresh_token: None,
            }),
            false,
        )
        .await
    }

    pub async fn revoke_token(&self) -> Result<MessageResponse, SSOError> {
        let token = match self.access_token().await {
            Some(t) => t,
            None => return Ok(MessageResponse { message: "no token to revoke".to_string() }),
        };

        let resp: MessageResponse = self
            .request(
                Method::POST,
                "/api/v1/token/revoke",
                Some(&RevokeRequest { token }),
                false,
            )
            .await?;

        self.clear_tokens().await;
        Ok(resp)
    }

    pub async fn forgot_password(&self, email: &str) -> Result<MessageResponse, SSOError> {
        self.request(
            Method::POST,
            "/api/v1/forgot-password",
            Some(&serde_json::json!({ "email": email })),
            false,
        )
        .await
    }

    pub async fn reset_password(
        &self,
        token: &str,
        user_id: &str,
        new_password: &str,
    ) -> Result<MessageResponse, SSOError> {
        self.request(
            Method::POST,
            "/api/v1/reset-password",
            Some(&serde_json::json!({
                "token": token,
                "user_id": user_id,
                "new_password": new_password,
            })),
            false,
        )
        .await
    }

    pub async fn verify_email(
        &self,
        token: &str,
        user_id: &str,
    ) -> Result<MessageResponse, SSOError> {
        let path = format!(
            "/api/v1/verify-email?token={}&user_id={}",
            url_encode(token),
            url_encode(user_id)
        );
        self.request(Method::GET, &path, None::<&()>, false).await
    }

    pub async fn send_verification_email(&self) -> Result<MessageResponse, SSOError> {
        self.request(Method::POST, "/api/v1/verify-email/send", None::<&()>, true)
            .await
    }

    // =======================================================================
    // 用户
    // =======================================================================

    pub async fn user_info(&self) -> Result<UserInfo, SSOError> {
        self.request(Method::GET, "/api/v1/userinfo", None::<&()>, true)
            .await
    }

    pub async fn change_password(
        &self,
        old_password: &str,
        new_password: &str,
    ) -> Result<MessageResponse, SSOError> {
        self.request(
            Method::POST,
            "/api/v1/change-password",
            Some(&ChangePasswordRequest {
                old_password: old_password.to_string(),
                new_password: new_password.to_string(),
            }),
            true,
        )
        .await
    }

    // =======================================================================
    // OAuth2
    // =======================================================================

    pub async fn authorize(
        &self,
        client_id: &str,
        redirect_uri: &str,
        scope: &str,
        state: &str,
    ) -> Result<AuthorizeResponse, SSOError> {
        let path = format!(
            "/api/v1/authorize?client_id={}&redirect_uri={}&response_type=code&scope={}&state={}",
            url_encode(client_id),
            url_encode(redirect_uri),
            url_encode(scope),
            url_encode(state),
        );
        self.request(Method::GET, &path, None::<&()>, true).await
    }

    // =======================================================================
    // MFA
    // =======================================================================

    pub async fn mfa_setup(&self) -> Result<MFASetupResponse, SSOError> {
        self.request(Method::POST, "/api/v1/mfa/setup", None::<&()>, true)
            .await
    }

    pub async fn mfa_verify(&self, code: &str) -> Result<MessageResponse, SSOError> {
        self.request(
            Method::POST,
            "/api/v1/mfa/verify",
            Some(&MFAVerifyRequest {
                code: code.to_string(),
            }),
            true,
        )
        .await
    }

    pub async fn mfa_disable(&self, code: &str) -> Result<MessageResponse, SSOError> {
        self.request(
            Method::POST,
            "/api/v1/mfa/disable",
            Some(&MFAVerifyRequest {
                code: code.to_string(),
            }),
            true,
        )
        .await
    }

    pub async fn mfa_status(&self) -> Result<MFAStatusResponse, SSOError> {
        self.request(Method::GET, "/api/v1/mfa/status", None::<&()>, true)
            .await
    }

    // =======================================================================
    // 管理员
    // =======================================================================

    pub async fn admin_health(&self) -> Result<HealthResponse, SSOError> {
        self.request(Method::GET, "/admin/health", None::<&()>, true)
            .await
    }

    pub async fn admin_cleanup(&self) -> Result<MessageResponse, SSOError> {
        self.request(Method::POST, "/admin/cleanup", None::<&()>, true)
            .await
    }

    pub async fn list_users(
        &self,
        page: u64,
        page_size: u64,
    ) -> Result<UserListResponse, SSOError> {
        let path = format!("/admin/users?page={page}&pageSize={page_size}");
        self.request(Method::GET, &path, None::<&()>, true).await
    }

    pub async fn get_user(&self, user_id: &str) -> Result<UserItem, SSOError> {
        let path = format!("/admin/users?id={}", url_encode(user_id));
        self.request(Method::GET, &path, None::<&()>, true).await
    }

    pub async fn disable_user(&self, user_id: &str) -> Result<MessageResponse, SSOError> {
        self.request(
            Method::POST,
            "/admin/users/disable",
            Some(&DisableUserRequest {
                user_id: user_id.to_string(),
            }),
            true,
        )
        .await
    }

    pub async fn enable_user(&self, user_id: &str) -> Result<MessageResponse, SSOError> {
        self.request(
            Method::POST,
            "/admin/users/enable",
            Some(&DisableUserRequest {
                user_id: user_id.to_string(),
            }),
            true,
        )
        .await
    }

    // =======================================================================
    // OIDC
    // =======================================================================

    pub async fn discovery(&self) -> Result<DiscoveryResponse, SSOError> {
        self.request(
            Method::GET,
            "/.well-known/openid-configuration",
            None::<&()>,
            false,
        )
        .await
    }

    pub async fn jwks(&self) -> Result<JWKSResponse, SSOError> {
        self.request(Method::GET, "/.well-known/jwks.json", None::<&()>, false)
            .await
    }
}

/// URL 编码（简单实现）
fn url_encode(s: &str) -> String {
    s.replace('%', "%25")
        .replace(' ', "%20")
        .replace('?', "%3F")
        .replace('&', "%26")
        .replace('=', "%3D")
        .replace('+', "%2B")
        .replace('#', "%23")
}
