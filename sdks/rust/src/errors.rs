use std::fmt;

/// SSO 错误码
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ErrorCode {
    Internal,
    BadRequest,
    NotFound,
    Conflict,
    Unauthorized,
    Forbidden,
    TooManyRequests,
    InvalidCredentials,
    AccountLocked,
    AccountDisabled,
    InvalidToken,
    TokenExpired,
    EmailExists,
    EmailInvalid,
    EmailRequired,
    PasswordTooShort,
    PasswordTooLong,
    PasswordRequired,
    InvalidRequestFormat,
    RequestBodyTooLarge,

    // === 阶段 5 SDK 同步：服务端阶段 2/3/4 引入的错误码 ===

    // Token 轮换 / 重放（阶段 2.1）
    // Refresh Token 已被使用过又再次出现，重放攻击典型特征
    // SDK 收到此错误应清空本地 Token 并要求用户重新登录
    TokenRotated,

    // OAuth Scope / PKCE / Consent（阶段 2.2）
    InvalidScope,      // scope 超出客户端允许或白名单
    PKCERequired,      // 公共客户端必须使用 PKCE（S256）
    ConsentRequired,   // 需要用户同意授权
    ConsentDenied,     // 用户拒绝授权
    ConsentInvalid,    // consent_token 无效或已过期
    ClientMismatch,    // refresh_token 客户端归属不一致

    // MFA 两阶段登录（阶段 2.x）
    MFAChallengeInvalid,    // Challenge 无效或已被使用
    MFAChallengeExpired,    // Challenge 已过期
    InvalidMFACode,         // TOTP 或恢复码无效
    TooManyMFAAttempts,     // 尝试次数过多（默认 5 次）
    MFAServiceUnavailable,  // MFA 服务未装配

    // Social Login 基础（阶段 2.2 改造）
    ProviderNotSupported,
    OAuthCodeExchangeFailed,
    SocialLoginFailed,
    OAuthStateInvalid,
    OAuthStateExpired,

    // Social Login 安全增强（阶段 2.3 新增）
    ProviderEmailNotVerified,
    SocialAccountConflict,
    EmailConflictWithLocal,
    ProviderUserIDMissing,

    // 邮件（阶段 4.3）
    // 服务端 SMTP 错误统一返回此通用错误码，不暴露 SMTP 内部信息
    EmailSendFailed,

    Other(String),
}

impl fmt::Display for ErrorCode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Internal => write!(f, "INTERNAL_ERROR"),
            Self::BadRequest => write!(f, "BAD_REQUEST"),
            Self::NotFound => write!(f, "NOT_FOUND"),
            Self::Conflict => write!(f, "CONFLICT"),
            Self::Unauthorized => write!(f, "UNAUTHORIZED"),
            Self::Forbidden => write!(f, "FORBIDDEN"),
            Self::TooManyRequests => write!(f, "TOO_MANY_REQUESTS"),
            Self::InvalidCredentials => write!(f, "INVALID_CREDENTIALS"),
            Self::AccountLocked => write!(f, "ACCOUNT_LOCKED"),
            Self::AccountDisabled => write!(f, "ACCOUNT_DISABLED"),
            Self::InvalidToken => write!(f, "INVALID_TOKEN"),
            Self::TokenExpired => write!(f, "TOKEN_EXPIRED"),
            Self::EmailExists => write!(f, "EMAIL_EXISTS"),
            Self::EmailInvalid => write!(f, "EMAIL_INVALID"),
            Self::EmailRequired => write!(f, "EMAIL_REQUIRED"),
            Self::PasswordTooShort => write!(f, "PASSWORD_TOO_SHORT"),
            Self::PasswordTooLong => write!(f, "PASSWORD_TOO_LONG"),
            Self::PasswordRequired => write!(f, "PASSWORD_REQUIRED"),
            Self::InvalidRequestFormat => write!(f, "INVALID_REQUEST_FORMAT"),
            Self::RequestBodyTooLarge => write!(f, "REQUEST_BODY_TOO_LARGE"),
            Self::TokenRotated => write!(f, "TOKEN_ROTATED"),
            Self::InvalidScope => write!(f, "INVALID_SCOPE"),
            Self::PKCERequired => write!(f, "PKCE_REQUIRED"),
            Self::ConsentRequired => write!(f, "CONSENT_REQUIRED"),
            Self::ConsentDenied => write!(f, "CONSENT_DENIED"),
            Self::ConsentInvalid => write!(f, "CONSENT_INVALID"),
            Self::ClientMismatch => write!(f, "CLIENT_MISMATCH"),
            Self::MFAChallengeInvalid => write!(f, "MFA_CHALLENGE_INVALID"),
            Self::MFAChallengeExpired => write!(f, "MFA_CHALLENGE_EXPIRED"),
            Self::InvalidMFACode => write!(f, "INVALID_MFA_CODE"),
            Self::TooManyMFAAttempts => write!(f, "TOO_MANY_MFA_ATTEMPTS"),
            Self::MFAServiceUnavailable => write!(f, "MFA_SERVICE_UNAVAILABLE"),
            Self::ProviderNotSupported => write!(f, "PROVIDER_NOT_SUPPORTED"),
            Self::OAuthCodeExchangeFailed => write!(f, "OAUTH_CODE_EXCHANGE_FAILED"),
            Self::SocialLoginFailed => write!(f, "SOCIAL_LOGIN_FAILED"),
            Self::OAuthStateInvalid => write!(f, "OAUTH_STATE_INVALID"),
            Self::OAuthStateExpired => write!(f, "OAUTH_STATE_EXPIRED"),
            Self::ProviderEmailNotVerified => write!(f, "PROVIDER_EMAIL_NOT_VERIFIED"),
            Self::SocialAccountConflict => write!(f, "SOCIAL_ACCOUNT_CONFLICT"),
            Self::EmailConflictWithLocal => write!(f, "EMAIL_CONFLICT_WITH_LOCAL"),
            Self::ProviderUserIDMissing => write!(f, "PROVIDER_USER_ID_MISSING"),
            Self::EmailSendFailed => write!(f, "EMAIL_SEND_FAILED"),
            Self::Other(s) => write!(f, "{s}"),
        }
    }
}

impl ErrorCode {
    pub fn from_str(s: &str) -> Self {
        match s {
            "INTERNAL_ERROR" => Self::Internal,
            "BAD_REQUEST" => Self::BadRequest,
            "NOT_FOUND" => Self::NotFound,
            "CONFLICT" => Self::Conflict,
            "UNAUTHORIZED" => Self::Unauthorized,
            "FORBIDDEN" => Self::Forbidden,
            "TOO_MANY_REQUESTS" => Self::TooManyRequests,
            "INVALID_CREDENTIALS" => Self::InvalidCredentials,
            "ACCOUNT_LOCKED" => Self::AccountLocked,
            "ACCOUNT_DISABLED" => Self::AccountDisabled,
            "INVALID_TOKEN" => Self::InvalidToken,
            "TOKEN_EXPIRED" => Self::TokenExpired,
            "EMAIL_EXISTS" => Self::EmailExists,
            "EMAIL_INVALID" => Self::EmailInvalid,
            "EMAIL_REQUIRED" => Self::EmailRequired,
            "PASSWORD_TOO_SHORT" => Self::PasswordTooShort,
            "PASSWORD_TOO_LONG" => Self::PasswordTooLong,
            "PASSWORD_REQUIRED" => Self::PasswordRequired,
            "INVALID_REQUEST_FORMAT" => Self::InvalidRequestFormat,
            "REQUEST_BODY_TOO_LARGE" => Self::RequestBodyTooLarge,
            "TOKEN_ROTATED" => Self::TokenRotated,
            "INVALID_SCOPE" => Self::InvalidScope,
            "PKCE_REQUIRED" => Self::PKCERequired,
            "CONSENT_REQUIRED" => Self::ConsentRequired,
            "CONSENT_DENIED" => Self::ConsentDenied,
            "CONSENT_INVALID" => Self::ConsentInvalid,
            "CLIENT_MISMATCH" => Self::ClientMismatch,
            "MFA_CHALLENGE_INVALID" => Self::MFAChallengeInvalid,
            "MFA_CHALLENGE_EXPIRED" => Self::MFAChallengeExpired,
            "INVALID_MFA_CODE" => Self::InvalidMFACode,
            "TOO_MANY_MFA_ATTEMPTS" => Self::TooManyMFAAttempts,
            "MFA_SERVICE_UNAVAILABLE" => Self::MFAServiceUnavailable,
            "PROVIDER_NOT_SUPPORTED" => Self::ProviderNotSupported,
            "OAUTH_CODE_EXCHANGE_FAILED" => Self::OAuthCodeExchangeFailed,
            "SOCIAL_LOGIN_FAILED" => Self::SocialLoginFailed,
            "OAUTH_STATE_INVALID" => Self::OAuthStateInvalid,
            "OAUTH_STATE_EXPIRED" => Self::OAuthStateExpired,
            "PROVIDER_EMAIL_NOT_VERIFIED" => Self::ProviderEmailNotVerified,
            "SOCIAL_ACCOUNT_CONFLICT" => Self::SocialAccountConflict,
            "EMAIL_CONFLICT_WITH_LOCAL" => Self::EmailConflictWithLocal,
            "PROVIDER_USER_ID_MISSING" => Self::ProviderUserIDMissing,
            "EMAIL_SEND_FAILED" => Self::EmailSendFailed,
            other => Self::Other(other.to_string()),
        }
    }
}

/// SSO API 错误
#[derive(Debug, thiserror::Error)]
#[error("sso: {code} (HTTP {http_status}): {message}")]
pub struct SSOError {
    pub http_status: u16,
    pub code: ErrorCode,
    pub message: String,
    pub raw_body: String,
}

impl SSOError {
    pub fn is_not_found(&self) -> bool {
        self.http_status == 404
    }

    pub fn is_unauthorized(&self) -> bool {
        self.http_status == 401
    }

    pub fn is_forbidden(&self) -> bool {
        self.http_status == 403
    }

    pub fn is_conflict(&self) -> bool {
        self.http_status == 409
    }

    pub fn is_rate_limited(&self) -> bool {
        self.http_status == 429
    }

    // ========================================================================
    // 阶段 5.2 辅助判断方法
    //
    // 服务端阶段 2/3/4 引入了大量新的错误码，下游集成方若每次都通过字符串比较
    // 判断错误码可读性差且易错。这里提供一组语义化方法，覆盖最常见的安全处理分支。
    // ========================================================================

    /// Refresh Token 已被使用过（重放攻击特征）
    /// 收到此错误应立即清空本地 Token 并要求用户重新登录。
    pub fn is_token_rotated(&self) -> bool {
        self.code == ErrorCode::TokenRotated
    }

    /// 需要用户同意授权（CONSENT_REQUIRED 或 CONSENT_INVALID）
    /// 收到此错误应重新调用 authorize 获取 consent_token 并展示授权同意页面。
    pub fn is_consent_required(&self) -> bool {
        self.code == ErrorCode::ConsentRequired || self.code == ErrorCode::ConsentInvalid
    }

    /// 用户主动拒绝授权，应终止授权流程
    pub fn is_consent_denied(&self) -> bool {
        self.code == ErrorCode::ConsentDenied
    }

    /// 公共客户端必须使用 PKCE（S256），应生成 code_verifier 重新发起授权
    pub fn is_pkce_required(&self) -> bool {
        self.code == ErrorCode::PKCERequired
    }

    /// 请求的 scope 超出客户端允许范围或不在白名单
    pub fn is_invalid_scope(&self) -> bool {
        self.code == ErrorCode::InvalidScope
    }

    /// Refresh Token 与客户端归属不一致
    pub fn is_client_mismatch(&self) -> bool {
        self.code == ErrorCode::ClientMismatch
    }

    /// MFA Challenge 无效或已被使用，应重新触发登录
    pub fn is_mfa_challenge_invalid(&self) -> bool {
        self.code == ErrorCode::MFAChallengeInvalid
    }

    /// MFA Challenge 已过期，应重新触发登录
    pub fn is_mfa_challenge_expired(&self) -> bool {
        self.code == ErrorCode::MFAChallengeExpired
    }

    /// MFA 验证尝试次数过多（默认 5 次），challenge 已失效
    pub fn is_too_many_mfa_attempts(&self) -> bool {
        self.code == ErrorCode::TooManyMFAAttempts
    }

    /// 社交登录相关错误（统一处理）
    pub fn is_social_login_error(&self) -> bool {
        matches!(
            self.code,
            ErrorCode::ProviderNotSupported
                | ErrorCode::OAuthCodeExchangeFailed
                | ErrorCode::SocialLoginFailed
                | ErrorCode::OAuthStateInvalid
                | ErrorCode::OAuthStateExpired
                | ErrorCode::ProviderEmailNotVerified
                | ErrorCode::SocialAccountConflict
                | ErrorCode::EmailConflictWithLocal
                | ErrorCode::ProviderUserIDMissing
        )
    }

    /// 邮件发送失败（SMTP 错误统一返回此码，不暴露内部信息）
    pub fn is_email_send_failed(&self) -> bool {
        self.code == ErrorCode::EmailSendFailed
    }
}

#[derive(serde::Deserialize)]
struct ErrorBody {
    #[serde(default)]
    code: String,
    #[serde(default)]
    error: String,
    #[serde(default)]
    message: String,
}

pub fn parse_error(http_status: u16, body: &str) -> SSOError {
    let parsed: Result<ErrorBody, _> = serde_json::from_str(body);
    match parsed {
        Ok(e) => {
            let code_str = if e.code.is_empty() { e.error } else { e.code };
            SSOError {
                http_status,
                code: ErrorCode::from_str(&code_str),
                message: if e.message.is_empty() {
                    body.to_string()
                } else {
                    e.message
                },
                raw_body: body.to_string(),
            }
        }
        Err(_) => SSOError {
            http_status,
            code: ErrorCode::Other("UNKNOWN".to_string()),
            message: body.to_string(),
            raw_body: body.to_string(),
        },
    }
}
