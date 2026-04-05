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
}
