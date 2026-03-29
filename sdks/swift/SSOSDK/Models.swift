import Foundation

// ============================================================================
// 响应模型
// ============================================================================

public struct TokenResponse: Codable {
    public let accessToken: String
    public let refreshToken: String
    public let tokenType: String
    public let expiresIn: Int

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case refreshToken = "refresh_token"
        case tokenType = "token_type"
        case expiresIn = "expires_in"
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
