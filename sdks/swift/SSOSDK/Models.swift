import Foundation

// ============================================================================
// 响应模型
// ============================================================================

/// 登录/Token 响应
///
/// 阶段 5.4 契约扩展：MFA 两阶段登录
/// 当用户启用 MFA 时，服务端在第一阶段不返回 access_token/refresh_token（服务端 model
/// 使用 omitempty，JSON 中会完全省略这些字段），而是返回 mfa_required=true 与一次性
/// mfa_challenge 令牌（TTL 5 分钟）。
///
/// 为兼容此场景，本 struct 使用自定义 init(from:)：对 String 字段使用 decodeIfPresent
/// 并默认为空字符串，从而既能解码普通 Token 响应，也能解码 MFA 第一阶段响应。
/// 新增的 MFA 字段使用可选类型，调用方应通过 mfaRequired 判断是否需要第二阶段验证。
public struct TokenResponse: Codable {
    public let accessToken: String
    public let refreshToken: String
    public let tokenType: String
    public let expiresIn: Int
    // 阶段 5.4 契约扩展：MFA 两阶段登录字段
    public let mfaRequired: Bool?
    public let mfaChallenge: String?
    public let mfaMethods: [String]?

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case refreshToken = "refresh_token"
        case tokenType = "token_type"
        case expiresIn = "expires_in"
        case mfaRequired = "mfa_required"
        case mfaChallenge = "mfa_challenge"
        case mfaMethods = "mfa_methods"
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        // 服务端使用 omitempty，MFA 场景下这些字段会缺失，使用 decodeIfPresent 兜底
        accessToken = try c.decodeIfPresent(String.self, forKey: .accessToken) ?? ""
        refreshToken = try c.decodeIfPresent(String.self, forKey: .refreshToken) ?? ""
        tokenType = try c.decodeIfPresent(String.self, forKey: .tokenType) ?? ""
        // expires_in 在服务端始终返回（MFA 场景下为 challenge TTL）
        expiresIn = try c.decode(Int.self, forKey: .expiresIn)
        mfaRequired = try c.decodeIfPresent(Bool.self, forKey: .mfaRequired)
        mfaChallenge = try c.decodeIfPresent(String.self, forKey: .mfaChallenge)
        mfaMethods = try c.decodeIfPresent([String].self, forKey: .mfaMethods)
    }
}

public struct RegisterResponse: Codable {
    public let message: String
    public let data: RegisterData?
}

public struct RegisterData: Codable {
    public let userId: String
    public let email: String

    enum CodingKeys: String, CodingKey {
        case userId = "user_id"
        case email
    }
}

public struct UserInfo: Codable {
    public let sub: String
    public let email: String
    public let emailVerified: Bool

    enum CodingKeys: String, CodingKey {
        case sub, email
        case emailVerified = "email_verified"
    }
}

public struct MessageResponse: Codable {
    public let message: String
}

public struct MFASetupResponse: Codable {
    public let secret: String
    public let qrCodeUrl: String
    public let manualEntry: String

    enum CodingKeys: String, CodingKey {
        case secret
        case qrCodeUrl = "qr_code_url"
        case manualEntry = "manual_entry"
    }
}

public struct MFAStatusResponse: Codable {
    public let enabled: Bool
}

/// 授权响应
///
/// 阶段 5.3 契约修复：服务端 GET /api/v1/authorize 与 POST /api/v1/authorize/approve
/// 返回的响应结构不同。使用同一个 struct 并通过可选字段兼容两种场景：
///   - GET /authorize 返回：consentToken/clientID/redirectURI/scope/state/requireApproval
///     （code 为空，前端需展示授权同意页面）
///   - POST /authorize/approve 返回：code/state
///     （consentToken 等字段为空，客户端使用 code 调用 /token 端点换取 Access Token）
///
/// 集成方应根据 requireApproval 判断当前是处于"待同意"还是"已批准"状态。
public struct AuthorizeResponse: Codable {
    // GET /authorize 返回字段
    public let consentToken: String?
    public let clientID: String?
    public let redirectURI: String?
    public let scope: String?
    public let requireApproval: Bool?

    // POST /authorize/approve 返回字段
    public let code: String?
    public let state: String?

    enum CodingKeys: String, CodingKey {
        case consentToken = "consent_token"
        case clientID = "client_id"
        case redirectURI = "redirect_uri"
        case scope
        case requireApproval = "require_approval"
        case code, state
    }
}

/// 授权批准请求
///
/// 阶段 5.3 契约修复：服务端 /api/v1/authorize/approve 实际期望 {consent_token, state}。
/// 旧字段已通过 consent_token JWT 携带，请求体启用 DisallowUnknownFields。
public struct AuthorizeApproveRequest: Encodable {
    public let consentToken: String
    public let state: String

    enum CodingKeys: String, CodingKey {
        case consentToken = "consent_token"
        case state
    }

    public init(consentToken: String, state: String) {
        self.consentToken = consentToken
        self.state = state
    }
}

/// 授权拒绝请求
///
/// 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
public struct AuthorizeDenyRequest: Encodable {
    public let consentToken: String
    public let state: String

    enum CodingKeys: String, CodingKey {
        case consentToken = "consent_token"
        case state
    }

    public init(consentToken: String, state: String) {
        self.consentToken = consentToken
        self.state = state
    }
}

/// 授权拒绝响应
///
/// 阶段 5.3 新增：服务端返回 HTTP 403，error 固定为 "access_denied"。
/// SDK 不应将其视为成功响应；调用方拿到此响应后应向客户端应用回传
/// ?error=access_denied&state=xxx。
public struct AuthorizeDenyResponse: Codable {
    public let error: String
    public let errorDescription: String
    public let state: String

    enum CodingKeys: String, CodingKey {
        case error
        case errorDescription = "error_description"
        case state
    }
}

/// MFA 两阶段登录第二阶段验证请求
///
/// 阶段 5.4 契约扩展：当 login 返回 mfa_required=true 时，
/// 客户端使用返回的 mfa_challenge 调用本接口完成登录。
///
/// - method: "totp"（6 位数字验证码）或 "recovery_code"（恢复码字符串）
/// - code: 与 method 对应的验证值
///
/// 成功后服务端返回标准 TokenResponse（含 access_token/refresh_token）。
public struct LoginMFAVerifyRequest: Encodable {
    public let mfaChallenge: String
    public let method: String
    public let code: String

    enum CodingKeys: String, CodingKey {
        case mfaChallenge = "mfa_challenge"
        case method
        case code
    }

    public init(mfaChallenge: String, method: String, code: String) {
        self.mfaChallenge = mfaChallenge
        self.method = method
        self.code = code
    }
}

public struct HealthResponse: Codable {
    public let status: String
    public let timestamp: String
    public let database: String
    public let version: String
}

public struct UserListResponse: Codable {
    public let users: [UserItem]
    public let total: Int
    public let page: Int
    public let pageSize: Int
    public let totalPages: Int

    enum CodingKeys: String, CodingKey {
        case users, total, page
        case pageSize = "page_size"
        case totalPages = "total_pages"
    }
}

public struct UserItem: Codable {
    public let id: String
    public let email: String
    public let emailVerified: Bool
    public let mfaEnabled: Bool
    public let status: String

    enum CodingKeys: String, CodingKey {
        case id, email, status
        case emailVerified = "email_verified"
        case mfaEnabled = "mfa_enabled"
    }
}

public struct DiscoveryResponse: Codable {
    public let issuer: String
    public let tokenEndpoint: String
    public let jwksUri: String
    public let grantTypesSupported: [String]

    enum CodingKeys: String, CodingKey {
        case issuer
        case tokenEndpoint = "token_endpoint"
        case jwksUri = "jwks_uri"
        case grantTypesSupported = "grant_types_supported"
    }
}

public struct JWKSResponse: Codable {
    public let keys: [JWK]
}

public struct JWK: Codable {
    public let kty: String
    public let use: String
    public let kid: String
    public let n: String
    public let e: String
}

/// OAuth 提供商
///
/// 阶段 5.5 新增：社交登录提供商信息，由 GET /auth/providers 返回。
/// 服务端直接返回数组（不包裹在 data 中）。所有字段使用可选类型，
/// 以兼容服务端可能省略某些字段的情况（如 icon 缺失）。
public struct OAuthProvider: Codable {
    public let name: String?
    public let label: String?
    public let icon: String?
}
