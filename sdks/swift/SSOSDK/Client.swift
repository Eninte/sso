import Foundation

// ============================================================================
// SSOClient SSO 服务客户端
// ============================================================================

public actor SSOClient {
    private let baseURL: String
    private let session: URLSession
    private let decoder: JSONDecoder
    private let timeout: TimeInterval

    private var accessToken: String
    private var refreshToken: String
    private var tokenExpiry: Date

    public init(
        baseURL: String,
        accessToken: String = "",
        refreshToken: String = "",
        timeout: TimeInterval = 30
    ) {
        self.baseURL = baseURL.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        self.session = URLSession.shared
        self.decoder = JSONDecoder()
        self.timeout = timeout
        self.accessToken = accessToken
        self.refreshToken = refreshToken
        self.tokenExpiry = Date.distantFuture
    }

    public var currentAccessToken: String { accessToken }

    public func setTokens(accessToken: String, refreshToken: String, expiresIn: Int) {
        self.accessToken = accessToken
        self.refreshToken = refreshToken
        self.tokenExpiry = Date().addingTimeInterval(TimeInterval(expiresIn))
    }

    public func clearTokens() {
        accessToken = ""
        refreshToken = ""
    }

    // =======================================================================
    // HTTP 请求
    // =======================================================================

    private func request<T: Decodable>(
        method: String,
        path: String,
        body: Encodable? = nil,
        auth: Bool = false
    ) async throws -> T {
        guard let url = URL(string: "\(baseURL)\(path)") else {
            throw SSOError(httpStatus: 0, code: "INVALID_URL", message: path, rawBody: "")
        }

        var req = URLRequest(url: url)
        req.httpMethod = method
        req.timeoutInterval = timeout
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")

        if auth {
            let token = try await ensureToken()
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        if let body = body {
            req.httpBody = try JSONEncoder().encode(body)
        }

        let (data, response) = try await session.data(for: req)
        let httpResponse = response as! HTTPURLResponse
        let text = String(data: data, encoding: .utf8) ?? ""

        if httpResponse.statusCode >= 400 {
            throw SSOError.parse(httpStatus: httpResponse.statusCode, body: text)
        }

        if text.isEmpty {
            return try decoder.decode(T.self, from: "{}".data(using: .utf8)!)
        }

        return try decoder.decode(T.self, from: data)
    }

    private func ensureToken() async throws -> String {
        guard !accessToken.isEmpty else {
            throw SSOError(httpStatus: 401, code: "UNAUTHORIZED", message: "no access token", rawBody: "")
        }

        let needsRefresh = Date() > tokenExpiry.addingTimeInterval(-30)
        if needsRefresh, !refreshToken.isEmpty {
            let resp: TokenResponse = try await request(
                method: "POST",
                path: "/api/v1/token",
                body: ["grant_type": "refresh_token", "refresh_token": refreshToken]
            )
            setTokens(accessToken: resp.accessToken, refreshToken: resp.refreshToken, expiresIn: resp.expiresIn)
            return resp.accessToken
        }

        return accessToken
    }

    // =======================================================================
    // 认证
    // =======================================================================

    public func register(email: String, password: String) async throws -> RegisterResponse {
        try await request(method: "POST", path: "/api/v1/register", body: ["email": email, "password": password])
    }

    public func login(email: String, password: String) async throws -> TokenResponse {
        let resp: TokenResponse = try await request(
            method: "POST", path: "/api/v1/login", body: ["email": email, "password": password]
        )
        setTokens(accessToken: resp.accessToken, refreshToken: resp.refreshToken, expiresIn: resp.expiresIn)
        return resp
    }

    public func refreshToken() async throws -> TokenResponse {
        guard !refreshToken.isEmpty else {
            throw SSOError(httpStatus: 401, code: "UNAUTHORIZED", message: "no refresh token", rawBody: "")
        }
        let resp: TokenResponse = try await request(
            method: "POST", path: "/api/v1/token",
            body: ["grant_type": "refresh_token", "refresh_token": refreshToken]
        )
        setTokens(accessToken: resp.accessToken, refreshToken: resp.refreshToken, expiresIn: resp.expiresIn)
        return resp
    }

    public func revokeToken() async throws -> MessageResponse {
        guard !accessToken.isEmpty else { return MessageResponse(message: "no token") }
        let resp: MessageResponse = try await request(
            method: "POST", path: "/api/v1/token/revoke", body: ["token": accessToken]
        )
        clearTokens()
        return resp
    }

    public func forgotPassword(email: String) async throws -> MessageResponse {
        try await request(method: "POST", path: "/api/v1/forgot-password", body: ["email": email])
    }

    public func resetPassword(token: String, userId: String, newPassword: String) async throws -> MessageResponse {
        try await request(
            method: "POST", path: "/api/v1/reset-password",
            body: ["token": token, "user_id": userId, "new_password": newPassword]
        )
    }

    // =======================================================================
    // 用户
    // =======================================================================

    public func userInfo() async throws -> UserInfo {
        try await request(method: "GET", path: "/api/v1/userinfo", auth: true)
    }

    public func changePassword(oldPassword: String, newPassword: String) async throws -> MessageResponse {
        try await request(
            method: "POST", path: "/api/v1/change-password",
            body: ["old_password": oldPassword, "new_password": newPassword], auth: true
        )
    }

    // =======================================================================
    // MFA
    // =======================================================================

    public func mfaSetup() async throws -> MFASetupResponse {
        try await request(method: "POST", path: "/api/v1/mfa/setup", auth: true)
    }

    public func mfaVerify(code: String) async throws -> MessageResponse {
        try await request(method: "POST", path: "/api/v1/mfa/verify", body: ["code": code], auth: true)
    }

    public func mfaDisable(code: String) async throws -> MessageResponse {
        try await request(method: "POST", path: "/api/v1/mfa/disable", body: ["code": code], auth: true)
    }

    public func mfaStatus() async throws -> MFAStatusResponse {
        try await request(method: "GET", path: "/api/v1/mfa/status", auth: true)
    }

    // =======================================================================
    // 管理员
    // =======================================================================

    public func adminHealth() async throws -> HealthResponse {
        try await request(method: "GET", path: "/admin/health", auth: true)
    }

    public func listUsers(page: Int = 1, pageSize: Int = 20) async throws -> UserListResponse {
        try await request(method: "GET", path: "/admin/users?page=\(page)&pageSize=\(pageSize)", auth: true)
    }

    public func disableUser(userId: String) async throws -> MessageResponse {
        try await request(method: "POST", path: "/admin/users/disable", body: ["user_id": userId], auth: true)
    }

    public func enableUser(userId: String) async throws -> MessageResponse {
        try await request(method: "POST", path: "/admin/users/enable", body: ["user_id": userId], auth: true)
    }

    // =======================================================================
    // OIDC
    // =======================================================================

    public func discovery() async throws -> DiscoveryResponse {
        try await request(method: "GET", path: "/.well-known/openid-configuration")
    }

    public func jwks() async throws -> JWKSResponse {
        try await request(method: "GET", path: "/.well-known/jwks.json")
    }
}
