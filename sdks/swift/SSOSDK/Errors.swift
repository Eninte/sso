import Foundation

// ============================================================================
// SSOError 错误类型
// ============================================================================

public struct SSOError: Error, LocalizedError {
    public let httpStatus: Int
    public let code: String
    public let message: String
    public let rawBody: String

    public var errorDescription: String? {
        "sso: \(code) (HTTP \(httpStatus)): \(message)"
    }

    public func isNotFound() -> Bool { httpStatus == 404 }
    public func isUnauthorized() -> Bool { httpStatus == 401 }
    public func isForbidden() -> Bool { httpStatus == 403 }
    public func isConflict() -> Bool { httpStatus == 409 }
    public func isRateLimited() -> Bool { httpStatus == 429 }

    // ========================================================================
    // 阶段 5.2 辅助判断方法
    //
    // 服务端阶段 2/3/4 引入了大量新的错误码，下游集成方若每次都通过字符串比较
    // 判断错误码可读性差且易错。这里提供一组语义化方法，覆盖最常见的安全处理分支。
    // ========================================================================

    /// Refresh Token 已被使用过（重放攻击特征）
    /// 收到此错误应立即清空本地 Token 并要求用户重新登录。
    public func isTokenRotated() -> Bool { code == SSOErrorCode.tokenRotated }

    /// 需要用户同意授权（CONSENT_REQUIRED 或 CONSENT_INVALID）
    /// 收到此错误应重新调用 authorize 获取 consent_token 并展示授权同意页面。
    public func isConsentRequired() -> Bool {
        code == SSOErrorCode.consentRequired || code == SSOErrorCode.consentInvalid
    }

    /// 用户主动拒绝授权，应终止授权流程
    public func isConsentDenied() -> Bool { code == SSOErrorCode.consentDenied }

    /// 公共客户端必须使用 PKCE（S256），应生成 code_verifier 重新发起授权
    public func isPKCERequired() -> Bool { code == SSOErrorCode.pkceRequired }

    /// 请求的 scope 超出客户端允许范围或不在白名单
    public func isInvalidScope() -> Bool { code == SSOErrorCode.invalidScope }

    /// Refresh Token 与客户端归属不一致
    public func isClientMismatch() -> Bool { code == SSOErrorCode.clientMismatch }

    /// MFA Challenge 无效或已被使用，应重新触发登录
    public func isMFAChallengeInvalid() -> Bool { code == SSOErrorCode.mfaChallengeInvalid }

    /// MFA Challenge 已过期，应重新触发登录
    public func isMFAChallengeExpired() -> Bool { code == SSOErrorCode.mfaChallengeExpired }

    /// MFA 验证尝试次数过多（默认 5 次），challenge 已失效
    public func isTooManyMFAAttempts() -> Bool { code == SSOErrorCode.tooManyMFAAttempts }

    /// 社交登录相关错误（统一处理）
    public func isSocialLoginError() -> Bool {
        [
            SSOErrorCode.providerNotSupported,
            SSOErrorCode.oauthCodeExchangeFailed,
            SSOErrorCode.socialLoginFailed,
            SSOErrorCode.oauthStateInvalid,
            SSOErrorCode.oauthStateExpired,
            SSOErrorCode.providerEmailNotVerified,
            SSOErrorCode.socialAccountConflict,
            SSOErrorCode.emailConflictWithLocal,
            SSOErrorCode.providerUserIDMissing,
        ].contains(code)
    }

    /// 邮件发送失败（SMTP 错误统一返回此码，不暴露内部信息）
    public func isEmailSendFailed() -> Bool { code == SSOErrorCode.emailSendFailed }

    static func parse(httpStatus: Int, body: String) -> SSOError {
        guard let data = body.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return SSOError(httpStatus: httpStatus, code: "UNKNOWN", message: body, rawBody: body)
        }
        let code = (json["code"] as? String) ?? (json["error"] as? String) ?? ""
        let message = (json["message"] as? String) ?? body
        return SSOError(httpStatus: httpStatus, code: code, message: message, rawBody: body)
    }
}

public enum SSOErrorCode {
    public static let internal_ = "INTERNAL_ERROR"
    public static let badRequest = "BAD_REQUEST"
    public static let notFound = "NOT_FOUND"
    public static let conflict = "CONFLICT"
    public static let unauthorized = "UNAUTHORIZED"
    public static let forbidden = "FORBIDDEN"
    public static let tooManyRequests = "TOO_MANY_REQUESTS"
    public static let invalidCredentials = "INVALID_CREDENTIALS"
    public static let accountLocked = "ACCOUNT_LOCKED"
    public static let accountDisabled = "ACCOUNT_DISABLED"
    public static let invalidToken = "INVALID_TOKEN"
    public static let tokenExpired = "TOKEN_EXPIRED"
    public static let emailExists = "EMAIL_EXISTS"
    public static let emailInvalid = "EMAIL_INVALID"
    public static let emailRequired = "EMAIL_REQUIRED"
    public static let passwordTooShort = "PASSWORD_TOO_SHORT"
    public static let passwordTooLong = "PASSWORD_TOO_LONG"
    public static let passwordRequired = "PASSWORD_REQUIRED"
    public static let invalidRequestFormat = "INVALID_REQUEST_FORMAT"
    public static let requestBodyTooLarge = "REQUEST_BODY_TOO_LARGE"
    public static let missingAuthCode = "MISSING_AUTH_CODE" // 社交登录回调未携带 code 参数

    // === 阶段 5 SDK 同步：服务端阶段 2/3/4 引入的错误码 ===

    // Token 轮换 / 重放（阶段 2.1）
    // Refresh Token 已被使用过又再次出现，重放攻击典型特征
    // SDK 收到此错误应清空本地 Token 并要求用户重新登录
    public static let tokenRotated = "TOKEN_ROTATED"

    // OAuth Scope / PKCE / Consent（阶段 2.2）
    public static let invalidScope = "INVALID_SCOPE"            // scope 超出客户端允许或白名单
    public static let pkceRequired = "PKCE_REQUIRED"            // 公共客户端必须使用 PKCE（S256）
    public static let consentRequired = "CONSENT_REQUIRED"      // 需要用户同意授权
    public static let consentDenied = "CONSENT_DENIED"          // 用户拒绝授权
    public static let consentInvalid = "CONSENT_INVALID"        // consent_token 无效或已过期
    public static let clientMismatch = "CLIENT_MISMATCH"        // refresh_token 客户端归属不一致

    // MFA 两阶段登录（阶段 2.x）
    public static let mfaChallengeInvalid = "MFA_CHALLENGE_INVALID"      // Challenge 无效或已被使用
    public static let mfaChallengeExpired = "MFA_CHALLENGE_EXPIRED"      // Challenge 已过期
    public static let invalidMFACode = "INVALID_MFA_CODE"                 // TOTP 或恢复码无效
    public static let tooManyMFAAttempts = "TOO_MANY_MFA_ATTEMPTS"        // 尝试次数过多（默认 5 次）
    public static let mfaServiceUnavailable = "MFA_SERVICE_UNAVAILABLE"  // MFA 服务未装配

    // Social Login 基础（阶段 2.2 改造）
    public static let providerNotSupported = "PROVIDER_NOT_SUPPORTED"
    public static let oauthCodeExchangeFailed = "OAUTH_CODE_EXCHANGE_FAILED"
    public static let socialLoginFailed = "SOCIAL_LOGIN_FAILED"
    public static let oauthStateInvalid = "OAUTH_STATE_INVALID"
    public static let oauthStateExpired = "OAUTH_STATE_EXPIRED"

    // Social Login 安全增强（阶段 2.3 新增）
    public static let providerEmailNotVerified = "PROVIDER_EMAIL_NOT_VERIFIED"
    public static let socialAccountConflict = "SOCIAL_ACCOUNT_CONFLICT"
    public static let emailConflictWithLocal = "EMAIL_CONFLICT_WITH_LOCAL"
    public static let providerUserIDMissing = "PROVIDER_USER_ID_MISSING"

    // 邮件（阶段 4.3）
    // 服务端 SMTP 错误统一返回此通用错误码，不暴露 SMTP 内部信息
    public static let emailSendFailed = "EMAIL_SEND_FAILED"
}
