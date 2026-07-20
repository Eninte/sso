import Foundation

// ============================================================================
// SSOClient SSO 服务客户端
// ============================================================================

/// 服务端 handlerutil.WriteJSONSuccess 返回的 {"data":{...}} 包装
/// 仅用于剥离 data 字段，不应直接暴露给调用方。
private struct DataWrapper<T: Decodable>: Decodable {
    let data: T
}

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

    /// 用户登录（第一阶段）
    ///
    /// 阶段 5.4 契约扩展：当用户启用 MFA 时，服务端返回 mfa_required=true 与
    /// 一次性 mfa_challenge 令牌（TTL 5 分钟），此时 access_token/refresh_token 为空。
    /// 本方法在这种情况下不会调用 setTokens；调用方应检查 resp.mfaRequired，
    /// 若为 true 则提示用户输入 MFA 验证码并调用 verifyMFALogin 完成第二阶段登录。
    public func login(email: String, password: String) async throws -> TokenResponse {
        let resp: TokenResponse = try await request(
            method: "POST", path: "/api/v1/login", body: ["email": email, "password": password]
        )
        if resp.mfaRequired != true {
            setTokens(accessToken: resp.accessToken, refreshToken: resp.refreshToken, expiresIn: resp.expiresIn)
        }
        return resp
    }

    /// MFA 两阶段登录第二阶段验证
    ///
    /// 阶段 5.4 契约扩展：POST /api/v1/login/mfa/verify
    /// 使用 login 返回的 mfaChallenge 与用户输入的验证码完成登录。
    /// 成功后服务端返回标准 TokenResponse，本方法会调用 setTokens 持久化。
    ///
    /// 失败错误码：MFA_CHALLENGE_INVALID / MFA_CHALLENGE_EXPIRED /
    /// INVALID_MFA_CODE / TOO_MANY_MFA_ATTEMPTS / MFA_SERVICE_UNAVAILABLE
    public func verifyMFALogin(req: LoginMFAVerifyRequest) async throws -> TokenResponse {
        let resp: TokenResponse = try await request(
            method: "POST", path: "/api/v1/login/mfa/verify", body: req
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
    // OAuth2
    // =======================================================================

    /// 获取 OAuth2 授权（consent_token）
    ///
    /// 阶段 5.3 契约修复：服务端 GET /api/v1/authorize 返回 {consent_token, client_id,
    /// redirect_uri, scope, state, require_approval}，不再直接返回 code。
    /// 调用方应展示授权同意页面，用户同意后调用 approveAuthorization 获取 code。
    public func authorize(
        clientID: String, redirectURI: String, scope: String, state: String
    ) async throws -> AuthorizeResponse {
        var components = URLComponents(string: "/api/v1/authorize")!
        components.queryItems = [
            URLQueryItem(name: "client_id", value: clientID),
            URLQueryItem(name: "redirect_uri", value: redirectURI),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: scope),
            URLQueryItem(name: "state", value: state),
        ]
        let path = components.url!.relativeString
        return try await request(method: "GET", path: path, auth: true)
    }

    /// 获取 OAuth2 授权（带 PKCE，consent_token）
    ///
    /// 阶段 5.3 新增：公共客户端必须使用 PKCE（S256）。
    public func authorizeWithPKCE(
        clientID: String, redirectURI: String, scope: String, state: String, codeChallenge: String
    ) async throws -> AuthorizeResponse {
        var components = URLComponents(string: "/api/v1/authorize")!
        components.queryItems = [
            URLQueryItem(name: "client_id", value: clientID),
            URLQueryItem(name: "redirect_uri", value: redirectURI),
            URLQueryItem(name: "response_type", value: "code"),
            URLQueryItem(name: "scope", value: scope),
            URLQueryItem(name: "state", value: state),
            URLQueryItem(name: "code_challenge", value: codeChallenge),
            URLQueryItem(name: "code_challenge_method", value: "S256"),
        ]
        let path = components.url!.relativeString
        return try await request(method: "GET", path: path, auth: true)
    }

    /// 批准 OAuth2 授权
    ///
    /// 阶段 5.3 新增：服务端期望请求体 {consent_token, state}，
    /// 不再接受 client_id/redirect_uri/scope 等字段（consent_token JWT 内部已携带）。
    /// 调用方需先调用 authorize/authorizeWithPKCE 获取 consentToken，再传给本方法。
    ///
    /// 成功后返回 {code, state}，使用 code 调用 /api/v1/token 换取 Access Token。
    public func approveAuthorization(req: AuthorizeApproveRequest) async throws -> AuthorizeResponse {
        try await request(method: "POST", path: "/api/v1/authorize/approve", body: req, auth: true)
    }

    /// 拒绝 OAuth2 授权
    ///
    /// 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
    /// 服务端固定返回 HTTP 403 + {error:"access_denied", error_description, state}，
    /// 本方法将此响应当作正常的 DenyResponse 返回（不视为错误），
    /// 调用方拿到后应向客户端应用回传 ?error=access_denied&state=xxx。
    ///
    /// 注意：仅在用户主动拒绝时调用；其他场景的 403 仍按错误处理。
    public func denyAuthorization(req: AuthorizeDenyRequest) async throws -> AuthorizeDenyResponse {
        do {
            return try await request(method: "POST", path: "/api/v1/authorize/deny", body: req, auth: true)
        } catch let error as SSOError where error.httpStatus == 403 {
            if let data = error.rawBody.data(using: .utf8),
               let resp = try? decoder.decode(AuthorizeDenyResponse.self, from: data) {
                return resp
            }
            throw error
        }
    }

    /// 用授权码换取 Access Token
    ///
    /// 阶段 B 审查修复：补齐 OAuth 完整流程缺失的环。
    /// 服务端 authorization_code grant 走 handlerutil.WriteJSONSuccess，
    /// 返回 {"data":{...}} 包裹格式（与 refresh_token grant 的平铺响应不同），
    /// 因此需先解码为 DataWrapper 再剥离 data 字段。
    ///
    /// 参考 internal/handler/token.go handleToken authorization_code 分支。
    public func exchangeCode(
        code: String,
        clientID: String,
        clientSecret: String,
        redirectURI: String,
        codeVerifier: String? = nil
    ) async throws -> TokenResponse {
        var body: [String: String] = [
            "grant_type": "authorization_code",
            "code": code,
            "client_id": clientID,
            "client_secret": clientSecret,
            "redirect_uri": redirectURI,
        ]
        if let v = codeVerifier { body["code_verifier"] = v }

        let wrapper: DataWrapper<TokenResponse> = try await request(
            method: "POST", path: "/api/v1/token", body: body
        )
        setTokens(accessToken: wrapper.data.accessToken,
                  refreshToken: wrapper.data.refreshToken,
                  expiresIn: wrapper.data.expiresIn)
        return wrapper.data
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
    public func getProviders() async throws -> [OAuthProvider] {
        try await request(method: "GET", path: "/auth/providers")
    }

    /// 构造发起社交登录的 URL
    ///
    /// 阶段 5.5 新增：直接构造 URL 字符串，不发起 HTTP 请求。
    /// 调用方应使用浏览器重定向到此 URL（服务端会返回 307 到 provider 授权页面），
    /// 而不是 SDK 直接 GET。
    ///
    /// - Parameters:
    ///   - provider: 社交登录提供商名称（如 "google" / "github"）
    ///   - state:    可选，CSRF 防护 state；为 nil 时由服务端自动生成 UUID
    /// - Returns: 完整的社交登录入口 URL
    public func getSocialLoginURL(provider: String, state: String?) -> String {
        let encoded = provider.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? provider
        var url = "\(baseURL)/auth/\(encoded)"
        if let s = state, !s.isEmpty {
            let encodedState = s.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? s
            url += "?state=\(encodedState)"
        }
        return url
    }

    /// 用回调返回的 code+state 完成社交登录
    ///
    /// 阶段 5.5 新增：调用 GET /auth/{provider}/callback?code={code}&state={state} 公开端点。
    /// 服务端直接平铺返回 TokenResponse（不包裹 data），无需认证。
    /// 成功后调用 setTokens 缓存到客户端。
    ///
    /// 失败错误码：MISSING_AUTH_CODE / OAUTH_STATE_INVALID / OAUTH_STATE_EXPIRED /
    /// PROVIDER_NOT_SUPPORTED / OAUTH_CODE_EXCHANGE_FAILED / SOCIAL_LOGIN_FAILED /
    /// PROVIDER_USER_ID_MISSING / PROVIDER_EMAIL_NOT_VERIFIED /
    /// SOCIAL_ACCOUNT_CONFLICT / EMAIL_CONFLICT_WITH_LOCAL / ACCOUNT_DISABLED / ACCOUNT_LOCKED
    public func exchangeSocialCode(provider: String, code: String, state: String) async throws -> TokenResponse {
        let encodedProvider = provider.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? provider
        let encodedCode = code.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? code
        let encodedState = state.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? state
        let path = "/auth/\(encodedProvider)/callback?code=\(encodedCode)&state=\(encodedState)"
        let resp: TokenResponse = try await request(method: "GET", path: path)
        setTokens(accessToken: resp.accessToken, refreshToken: resp.refreshToken, expiresIn: resp.expiresIn)
        return resp
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
        try await request(method: "GET", path: "/api/v1/admin/health", auth: true)
    }

    public func listUsers(page: Int = 1, pageSize: Int = 20) async throws -> UserListResponse {
        try await request(
            method: "GET",
            path: "/api/v1/admin/users?page=\(page)&pageSize=\(pageSize)",
            auth: true
        )
    }

    public func disableUser(userId: String) async throws -> MessageResponse {
        // userId 作为路径参数传递，需进行 URL 编码
        let encoded = userId.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? userId
        return try await request(
            method: "POST",
            path: "/api/v1/admin/users/\(encoded)/disable",
            auth: true
        )
    }

    public func enableUser(userId: String) async throws -> MessageResponse {
        // userId 作为路径参数传递，需进行 URL 编码
        let encoded = userId.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? userId
        return try await request(
            method: "POST",
            path: "/api/v1/admin/users/\(encoded)/enable",
            auth: true
        )
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
