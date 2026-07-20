use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};

use reqwest::{Client as HttpClient, Method};

use crate::errors::{parse_error, SSOError};
use crate::models::*;

/// 服务端 handlerutil.WriteJSONSuccess 返回的 {"data":{...}} 包装
/// 仅用于剥离 data 字段，不应直接暴露给调用方。
#[derive(serde::Deserialize)]
struct DataWrapper<T> {
    data: T,
}

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

        // 直接使用 http 客户端发送请求，避免与 request 方法形成递归
        let url = format!("{}/api/v1/token", self.base_url);
        let body = serde_json::json!({
            "grant_type": "refresh_token",
            "refresh_token": refresh_token,
        });

        let resp = self
            .http
            .post(&url)
            .json(&body)
            .send()
            .await
            .map_err(|e| SSOError {
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

        let token_resp: TokenResponse = serde_json::from_str(&text).map_err(|e| SSOError {
            http_status: status,
            code: crate::errors::ErrorCode::Internal,
            message: format!("parse token response: {e}"),
            raw_body: text,
        })?;

        self.set_tokens(&token_resp.access_token, &token_resp.refresh_token, token_resp.expires_in)
            .await;
        Ok(token_resp.access_token)
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

    /// 用户登录（第一阶段）
    ///
    /// 阶段 5.4 契约扩展：当用户启用 MFA 时，服务端返回 mfa_required=true 与
    /// 一次性 mfa_challenge 令牌（TTL 5 分钟），此时 access_token/refresh_token 为空。
    /// 本方法在这种情况下不会调用 set_tokens；调用方应检查 resp.mfa_required，
    /// 若为 true 则提示用户输入 MFA 验证码并调用 verify_mfa_login 完成第二阶段登录。
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

        if !resp.mfa_required {
            self.set_tokens(&resp.access_token, &resp.refresh_token, resp.expires_in)
                .await;
        }
        Ok(resp)
    }

    /// MFA 两阶段登录第二阶段验证
    ///
    /// 阶段 5.4 契约扩展：POST /api/v1/login/mfa/verify
    /// 使用 login 返回的 mfa_challenge 与用户输入的验证码完成登录。
    /// 成功后服务端返回标准 TokenResponse，本方法会调用 set_tokens 持久化。
    ///
    /// 失败错误码：MFA_CHALLENGE_INVALID / MFA_CHALLENGE_EXPIRED /
    /// INVALID_MFA_CODE / TOO_MANY_MFA_ATTEMPTS / MFA_SERVICE_UNAVAILABLE
    pub async fn verify_mfa_login(
        &self,
        req: &LoginMFAVerifyRequest,
    ) -> Result<TokenResponse, SSOError> {
        let resp: TokenResponse = self
            .request(Method::POST, "/api/v1/login/mfa/verify", Some(req), false)
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
        // 阶段 B 审查修复：服务端 authorization_code grant 走 handlerutil.WriteJSONSuccess，
        // 返回 {"data":{...}} 包裹格式（与 refresh_token grant 的平铺响应不同），需剥离 data 包装。
        // 参考 internal/handler/token.go: handleToken authorization_code 分支。
        let wrapper: DataWrapper<TokenResponse> = self
            .request(
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
            .await?;
        Ok(wrapper.data)
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

    /// 获取 OAuth2 授权（带 PKCE，consent_token）
    ///
    /// 阶段 5.3 新增：公共客户端必须使用 PKCE（S256）。
    pub async fn authorize_with_pkce(
        &self,
        client_id: &str,
        redirect_uri: &str,
        scope: &str,
        state: &str,
        code_challenge: &str,
    ) -> Result<AuthorizeResponse, SSOError> {
        let path = format!(
            "/api/v1/authorize?client_id={}&redirect_uri={}&response_type=code&scope={}&state={}&code_challenge={}&code_challenge_method=S256",
            url_encode(client_id),
            url_encode(redirect_uri),
            url_encode(scope),
            url_encode(state),
            url_encode(code_challenge),
        );
        self.request(Method::GET, &path, None::<&()>, true).await
    }

    /// 批准 OAuth2 授权
    ///
    /// 阶段 5.3 新增：服务端期望请求体 {consent_token, state}，
    /// 不再接受 client_id/redirect_uri/scope 等字段（consent_token JWT 内部已携带）。
    /// 调用方需先调用 authorize/authorize_with_pkce 获取 consent_token，再传给本方法。
    ///
    /// 成功后返回 {code, state}，使用 code 调用 /api/v1/token 换取 Access Token。
    pub async fn approve_authorization(
        &self,
        req: &AuthorizeApproveRequest,
    ) -> Result<AuthorizeResponse, SSOError> {
        self.request(Method::POST, "/api/v1/authorize/approve", Some(req), true)
            .await
    }

    /// 拒绝 OAuth2 授权
    ///
    /// 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
    /// 服务端固定返回 HTTP 403 + {error:"access_denied", error_description, state}，
    /// 本方法将此响应当作正常的 DenyResponse 返回（不视为错误），
    /// 调用方拿到后应向客户端应用回传 ?error=access_denied&state=xxx。
    ///
    /// 注意：仅在用户主动拒绝时调用；其他场景的 403 仍按错误处理。
    pub async fn deny_authorization(
        &self,
        req: &AuthorizeDenyRequest,
    ) -> Result<AuthorizeDenyResponse, SSOError> {
        match self
            .request::<AuthorizeDenyResponse>(Method::POST, "/api/v1/authorize/deny", Some(req), true)
            .await
        {
            Ok(resp) => Ok(resp),
            Err(err) => {
                // 服务端 deny 端点固定返回 403，解析错误响应体
                if err.http_status == 403 && !err.raw_body.is_empty() {
                    if let Ok(resp) = serde_json::from_str::<AuthorizeDenyResponse>(&err.raw_body) {
                        return Ok(resp);
                    }
                }
                Err(err)
            }
        }
    }

    // =======================================================================
    // Social Login 社交登录
    //
    // 阶段 5.5 新增：服务端契约
    //   - GET /auth/providers         公开端点，直接返回数组（不包裹 data）
    //   - GET /auth/{provider}?state= 公开端点，返回 HTTP 307 重定向到 provider 授权页面
    //   - GET /auth/{provider}/callback?code=&state= 公开端点，平铺返回 TokenResponse
    // =======================================================================

    /// 获取支持的社交登录提供商列表
    ///
    /// 阶段 5.5 新增：调用 GET /auth/providers 公开端点。
    /// 服务端直接返回数组（不包裹在 data 中），无需认证。
    pub async fn get_providers(&self) -> Result<Vec<OAuthProvider>, SSOError> {
        self.request::<Vec<OAuthProvider>>(Method::GET, "/auth/providers", None::<&()>, false)
            .await
    }

    /// 构造发起社交登录的 URL
    ///
    /// 阶段 5.5 新增：直接构造 URL 字符串，不发起 HTTP 请求。
    /// 调用方应使用浏览器重定向到此 URL（服务端会返回 307 到 provider 授权页面），
    /// 而不是 SDK 直接 GET。
    ///
    /// - `provider`: 社交登录提供商名称（如 "google" / "github"）
    /// - `state`:    可选，CSRF 防护 state；为 None 时由服务端自动生成 UUID
    pub fn get_social_login_url(&self, provider: &str, state: Option<&str>) -> String {
        let encoded = url_encode(provider);
        let mut url = format!("{}/auth/{}", self.base_url, encoded);
        if let Some(s) = state {
            if !s.is_empty() {
                url.push_str("?state=");
                url.push_str(&url_encode(s));
            }
        }
        url
    }

    /// 用回调返回的 code+state 完成社交登录
    ///
    /// 阶段 5.5 新增：调用 GET /auth/{provider}/callback?code={code}&state={state} 公开端点。
    /// 服务端直接平铺返回 TokenResponse（不包裹 data），无需认证。
    /// 成功后调用 set_tokens 缓存到客户端。
    ///
    /// 失败错误码：MISSING_AUTH_CODE / OAUTH_STATE_INVALID / OAUTH_STATE_EXPIRED /
    /// PROVIDER_NOT_SUPPORTED / OAUTH_CODE_EXCHANGE_FAILED / SOCIAL_LOGIN_FAILED /
    /// PROVIDER_USER_ID_MISSING / PROVIDER_EMAIL_NOT_VERIFIED /
    /// SOCIAL_ACCOUNT_CONFLICT / EMAIL_CONFLICT_WITH_LOCAL / ACCOUNT_DISABLED / ACCOUNT_LOCKED
    pub async fn exchange_social_code(
        &self,
        provider: &str,
        code: &str,
        state: &str,
    ) -> Result<TokenResponse, SSOError> {
        let path = format!(
            "/auth/{}/callback?code={}&state={}",
            url_encode(provider),
            url_encode(code),
            url_encode(state),
        );
        let resp: TokenResponse = self
            .request(Method::GET, &path, None::<&()>, false)
            .await?;

        self.set_tokens(&resp.access_token, &resp.refresh_token, resp.expires_in)
            .await;
        Ok(resp)
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
        self.request(Method::GET, "/api/v1/admin/health", None::<&()>, true)
            .await
    }

    pub async fn admin_cleanup(&self) -> Result<MessageResponse, SSOError> {
        self.request(Method::POST, "/api/v1/admin/cleanup", None::<&()>, true)
            .await
    }

    pub async fn list_users(
        &self,
        page: u64,
        page_size: u64,
    ) -> Result<UserListResponse, SSOError> {
        // 服务端使用 camelCase 的 pageSize 作为查询参数名
        let path = format!("/api/v1/admin/users?page={page}&pageSize={page_size}");
        self.request(Method::GET, &path, None::<&()>, true).await
    }

    pub async fn get_user(&self, user_id: &str) -> Result<UserItem, SSOError> {
        // 使用路径参数：GET /api/v1/admin/users/{id}
        let path = format!("/api/v1/admin/users/{}", url_encode(user_id));
        self.request(Method::GET, &path, None::<&()>, true).await
    }

    pub async fn disable_user(&self, user_id: &str) -> Result<MessageResponse, SSOError> {
        // 使用路径参数，无需请求体：POST /api/v1/admin/users/{id}/disable
        let path = format!("/api/v1/admin/users/{}/disable", url_encode(user_id));
        self.request(Method::POST, &path, None::<&()>, true).await
    }

    pub async fn enable_user(&self, user_id: &str) -> Result<MessageResponse, SSOError> {
        // 使用路径参数，无需请求体：POST /api/v1/admin/users/{id}/enable
        let path = format!("/api/v1/admin/users/{}/enable", url_encode(user_id));
        self.request(Method::POST, &path, None::<&()>, true).await
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

/// URL 编码（完整实现，符合RFC 3986）
fn url_encode(s: &str) -> String {
    let mut result = String::with_capacity(s.len() * 3);
    for byte in s.bytes() {
        match byte {
            // 未保留字符（unreserved characters）- 不需要编码
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'.' | b'_' | b'~' => {
                result.push(byte as char);
            }
            // 其他字符都需要百分号编码
            _ => {
                result.push_str(&format!("%{:02X}", byte));
            }
        }
    }
    result
}
