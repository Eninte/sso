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
