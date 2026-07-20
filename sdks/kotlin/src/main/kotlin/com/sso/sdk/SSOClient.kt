package com.sso.sdk

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import com.google.gson.reflect.TypeToken
import java.net.URLEncoder
import java.util.concurrent.TimeUnit

// ============================================================================
// 错误类型
// ============================================================================

data class SSOError(
    val httpStatus: Int,
    val code: String,
    val message: String = "",
    val rawBody: String = ""
) : Exception("sso: $code (HTTP $httpStatus): $message") {
    fun isNotFound() = httpStatus == 404
    fun isUnauthorized() = httpStatus == 401
    fun isForbidden() = httpStatus == 403
    fun isConflict() = httpStatus == 409
    fun isRateLimited() = httpStatus == 429

    // ========================================================================
    // 阶段 5.2 辅助判断方法
    //
    // 服务端阶段 2/3/4 引入了大量新的错误码，下游集成方若每次都通过字符串比较
    // 判断错误码可读性差且易错。这里提供一组语义化方法，覆盖最常见的安全处理分支。
    // ========================================================================

    /** Refresh Token 已被使用过（重放攻击特征）
     *  收到此错误应立即清空本地 Token 并要求用户重新登录 */
    fun isTokenRotated() = code == SSOErrorCode.TOKEN_ROTATED

    /** 需要用户同意授权（CONSENT_REQUIRED 或 CONSENT_INVALID） */
    fun isConsentRequired() =
        code == SSOErrorCode.CONSENT_REQUIRED || code == SSOErrorCode.CONSENT_INVALID

    /** 用户主动拒绝授权 */
    fun isConsentDenied() = code == SSOErrorCode.CONSENT_DENIED

    /** 公共客户端必须使用 PKCE（S256） */
    fun isPKCERequired() = code == SSOErrorCode.PKCE_REQUIRED

    /** 请求的 scope 超出客户端允许范围或不在白名单 */
    fun isInvalidScope() = code == SSOErrorCode.INVALID_SCOPE

    /** Refresh Token 与客户端归属不一致 */
    fun isClientMismatch() = code == SSOErrorCode.CLIENT_MISMATCH

    /** MFA Challenge 无效或已被使用 */
    fun isMFAChallengeInvalid() = code == SSOErrorCode.MFA_CHALLENGE_INVALID

    /** MFA Challenge 已过期 */
    fun isMFAChallengeExpired() = code == SSOErrorCode.MFA_CHALLENGE_EXPIRED

    /** MFA 验证尝试次数过多（默认 5 次） */
    fun isTooManyMFAAttempts() = code == SSOErrorCode.TOO_MANY_MFA_ATTEMPTS

    /** 社交登录相关错误（统一处理） */
    fun isSocialLoginError(): Boolean = code in setOf(
        SSOErrorCode.PROVIDER_NOT_SUPPORTED,
        SSOErrorCode.OAUTH_CODE_EXCHANGE_FAILED,
        SSOErrorCode.SOCIAL_LOGIN_FAILED,
        SSOErrorCode.OAUTH_STATE_INVALID,
        SSOErrorCode.OAUTH_STATE_EXPIRED,
        SSOErrorCode.PROVIDER_EMAIL_NOT_VERIFIED,
        SSOErrorCode.SOCIAL_ACCOUNT_CONFLICT,
        SSOErrorCode.EMAIL_CONFLICT_WITH_LOCAL,
        SSOErrorCode.PROVIDER_USER_ID_MISSING,
    )

    /** 邮件发送失败（SMTP 错误统一返回此码，不暴露内部信息） */
    fun isEmailSendFailed() = code == SSOErrorCode.EMAIL_SEND_FAILED

    companion object {
        fun parse(httpStatus: Int, body: String): SSOError {
            return try {
                val obj = Gson().fromJson(body, Map::class.java) as Map<*, *>
                val code = (obj["code"] ?: obj["error"] ?: "") as String
                val msg = (obj["message"] ?: body) as String
                SSOError(httpStatus, code, msg, body)
            } catch (_: Exception) {
                SSOError(httpStatus, "UNKNOWN", body, body)
            }
        }
    }
}

object SSOErrorCode {
    const val INTERNAL = "INTERNAL_ERROR"
    const val BAD_REQUEST = "BAD_REQUEST"
    const val NOT_FOUND = "NOT_FOUND"
    const val CONFLICT = "CONFLICT"
    const val UNAUTHORIZED = "UNAUTHORIZED"
    const val FORBIDDEN = "FORBIDDEN"
    const val TOO_MANY_REQUESTS = "TOO_MANY_REQUESTS"
    const val INVALID_CREDENTIALS = "INVALID_CREDENTIALS"
    const val ACCOUNT_LOCKED = "ACCOUNT_LOCKED"
    const val ACCOUNT_DISABLED = "ACCOUNT_DISABLED"
    const val INVALID_TOKEN = "INVALID_TOKEN"
    const val TOKEN_EXPIRED = "TOKEN_EXPIRED"
    const val EMAIL_EXISTS = "EMAIL_EXISTS"
    const val EMAIL_INVALID = "EMAIL_INVALID"
    const val EMAIL_REQUIRED = "EMAIL_REQUIRED"
    const val PASSWORD_TOO_SHORT = "PASSWORD_TOO_SHORT"
    const val PASSWORD_TOO_LONG = "PASSWORD_TOO_LONG"
    const val PASSWORD_REQUIRED = "PASSWORD_REQUIRED"
    const val INVALID_REQUEST_FORMAT = "INVALID_REQUEST_FORMAT"
    const val REQUEST_BODY_TOO_LARGE = "REQUEST_BODY_TOO_LARGE"

    // === 阶段 5 SDK 同步：服务端阶段 2/3/4 引入的错误码 ===

    // Token 轮换 / 重放（阶段 2.1）
    // Refresh Token 已被使用过又再次出现，重放攻击典型特征
    // SDK 收到此错误应清空本地 Token 并要求用户重新登录
    const val TOKEN_ROTATED = "TOKEN_ROTATED"

    // OAuth Scope / PKCE / Consent（阶段 2.2）
    const val INVALID_SCOPE = "INVALID_SCOPE"            // scope 超出客户端允许或白名单
    const val PKCE_REQUIRED = "PKCE_REQUIRED"             // 公共客户端必须使用 PKCE（S256）
    const val CONSENT_REQUIRED = "CONSENT_REQUIRED"      // 需要用户同意授权
    const val CONSENT_DENIED = "CONSENT_DENIED"           // 用户拒绝授权
    const val CONSENT_INVALID = "CONSENT_INVALID"         // consent_token 无效或已过期
    const val CLIENT_MISMATCH = "CLIENT_MISMATCH"         // refresh_token 客户端归属不一致

    // MFA 两阶段登录（阶段 2.x）
    const val MFA_CHALLENGE_INVALID = "MFA_CHALLENGE_INVALID"      // Challenge 无效或已被使用
    const val MFA_CHALLENGE_EXPIRED = "MFA_CHALLENGE_EXPIRED"      // Challenge 已过期
    const val INVALID_MFA_CODE = "INVALID_MFA_CODE"                // TOTP 或恢复码无效
    const val TOO_MANY_MFA_ATTEMPTS = "TOO_MANY_MFA_ATTEMPTS"        // 尝试次数过多（默认 5 次）
    const val MFA_SERVICE_UNAVAILABLE = "MFA_SERVICE_UNAVAILABLE"   // MFA 服务未装配

    // Social Login 基础（阶段 2.2 改造）
    const val PROVIDER_NOT_SUPPORTED = "PROVIDER_NOT_SUPPORTED"
    const val OAUTH_CODE_EXCHANGE_FAILED = "OAUTH_CODE_EXCHANGE_FAILED"
    const val SOCIAL_LOGIN_FAILED = "SOCIAL_LOGIN_FAILED"
    const val OAUTH_STATE_INVALID = "OAUTH_STATE_INVALID"
    const val OAUTH_STATE_EXPIRED = "OAUTH_STATE_EXPIRED"

    // Social Login 安全增强（阶段 2.3 新增）
    const val PROVIDER_EMAIL_NOT_VERIFIED = "PROVIDER_EMAIL_NOT_VERIFIED"
    const val SOCIAL_ACCOUNT_CONFLICT = "SOCIAL_ACCOUNT_CONFLICT"
    const val EMAIL_CONFLICT_WITH_LOCAL = "EMAIL_CONFLICT_WITH_LOCAL"
    const val PROVIDER_USER_ID_MISSING = "PROVIDER_USER_ID_MISSING"

    // 邮件（阶段 4.3）
    // 服务端 SMTP 错误统一返回此通用错误码，不暴露 SMTP 内部信息
    const val EMAIL_SEND_FAILED = "EMAIL_SEND_FAILED"
}

// ============================================================================
// 响应模型
// ============================================================================

/**
 * 登录/Token 响应
 *
 * 阶段 5.4 契约扩展：MFA 两阶段登录
 * 当用户启用 MFA 时，服务端在第一阶段不返回 access_token/refresh_token（服务端 model
 * 使用 omitempty），而是返回 mfa_required=true 与一次性 mfa_challenge 令牌（TTL 5 分钟）。
 * 调用方应通过 mfaRequired 判断是否需要第二阶段验证。
 */
data class TokenResponse(
    @SerializedName("access_token") val accessToken: String = "",
    @SerializedName("refresh_token") val refreshToken: String = "",
    @SerializedName("token_type") val tokenType: String = "Bearer",
    @SerializedName("expires_in") val expiresIn: Int = 0,
    // 阶段 5.4 契约扩展：MFA 两阶段登录字段（服务端 omitempty，使用可空类型）
    @SerializedName("mfa_required") val mfaRequired: Boolean? = null,
    @SerializedName("mfa_challenge") val mfaChallenge: String? = null,
    @SerializedName("mfa_methods") val mfaMethods: List<String>? = null
)

/**
 * MFA 两阶段登录第二阶段验证请求
 *
 * 阶段 5.4 契约扩展：当 login 返回 mfa_required=true 时，
 * 客户端使用返回的 mfa_challenge 调用本接口完成登录。
 *
 * - method: "totp"（6 位数字验证码）或 "recovery_code"（恢复码字符串）
 * - code: 与 method 对应的验证值
 *
 * 成功后服务端返回标准 TokenResponse（含 access_token/refresh_token）。
 */
data class LoginMFAVerifyRequest(
    @SerializedName("mfa_challenge") val mfaChallenge: String,
    val method: String,  // "totp" 或 "recovery_code"
    val code: String
)

data class RegisterResponse(
    val message: String,
    val data: RegisterData? = null
)

data class RegisterData(
    @SerializedName("user_id") val userId: String,
    val email: String
)

data class UserInfo(
    val sub: String,
    val email: String,
    @SerializedName("email_verified") val emailVerified: Boolean = false
)

data class MessageResponse(val message: String = "")

data class MFASetupResponse(
    val secret: String = "",
    @SerializedName("qr_code_url") val qrCodeUrl: String = "",
    @SerializedName("manual_entry") val manualEntry: String = ""
)

data class MFAStatusResponse(val enabled: Boolean = false)

data class HealthResponse(
    val status: String = "",
    val timestamp: String = "",
    val database: String = "",
    val version: String = ""
)

data class UserListResponse(
    val users: List<UserItem> = emptyList(),
    val total: Int = 0,
    val page: Int = 1,
    @SerializedName("page_size") val pageSize: Int = 20,
    @SerializedName("total_pages") val totalPages: Int = 0
)

data class UserItem(
    val id: String = "",
    val email: String = "",
    @SerializedName("email_verified") val emailVerified: Boolean = false,
    @SerializedName("mfa_enabled") val mfaEnabled: Boolean = false,
    val status: String = ""
)

data class DiscoveryResponse(
    val issuer: String = "",
    @SerializedName("token_endpoint") val tokenEndpoint: String = "",
    @SerializedName("jwks_uri") val jwksUri: String = "",
    @SerializedName("grant_types_supported") val grantTypesSupported: List<String> = emptyList()
)

data class JWK(
    val kty: String = "",
    @SerializedName("use") val use: String = "",
    val kid: String = "",
    val n: String = "",
    val e: String = ""
)

data class JWKSResponse(val keys: List<JWK> = emptyList())

/**
 * OAuth 提供商
 *
 * 阶段 5.5 新增：社交登录提供商信息，由 GET /auth/providers 返回。
 * 服务端直接返回数组（不包裹在 data 中）。所有字段使用可空类型 + 默认值，
 * 以兼容服务端可能省略某些字段的情况（如 icon 缺失）。
 */
data class OAuthProvider(
    val name: String = "",
    val label: String = "",
    val icon: String = ""
)

/**
 * 授权响应
 *
 * 阶段 5.3 契约修复：服务端 GET /api/v1/authorize 与 POST /api/v1/authorize/approve
 * 返回的响应结构不同。使用同一个 data class 通过可空字段兼容两种场景：
 *   - GET /authorize 返回：consentToken/clientId/redirectUri/scope/state/requireApproval
 *     （code 为空，前端需展示授权同意页面）
 *   - POST /authorize/approve 返回：code/state
 *     （consentToken 等字段为空，客户端使用 code 调用 /token 端点换取 Access Token）
 *
 * 集成方应根据 requireApproval 判断当前是处于"待同意"还是"已批准"状态。
 */
data class AuthorizeResponse(
    @SerializedName("consent_token") val consentToken: String? = null,
    @SerializedName("client_id") val clientId: String? = null,
    @SerializedName("redirect_uri") val redirectUri: String? = null,
    val scope: String? = null,
    @SerializedName("require_approval") val requireApproval: Boolean? = null,
    val code: String? = null,
    val state: String? = null
)

/**
 * 授权批准请求
 *
 * 阶段 5.3 契约修复：服务端 /api/v1/authorize/approve 实际期望 {consent_token, state}。
 * 旧字段已通过 consent_token JWT 携带，请求体启用 DisallowUnknownFields。
 */
data class AuthorizeApproveRequest(
    @SerializedName("consent_token") val consentToken: String,
    val state: String
)

/**
 * 授权拒绝请求
 *
 * 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
 */
data class AuthorizeDenyRequest(
    @SerializedName("consent_token") val consentToken: String,
    val state: String
)

/**
 * 授权拒绝响应
 *
 * 阶段 5.3 新增：服务端返回 HTTP 403，error 固定为 "access_denied"。
 * SDK 不应将其视为成功响应；调用方拿到此响应后应向客户端应用回传
 * ?error=access_denied&state=xxx。
 */
data class AuthorizeDenyResponse(
    val error: String = "",
    @SerializedName("error_description") val errorDescription: String = "",
    val state: String = ""
)

// ============================================================================
// SSOClient 客户端
// ============================================================================

class SSOClient(
    private val baseUrl: String,
    accessToken: String = "",
    refreshToken: String = "",
    timeoutSeconds: Long = 30
) {
    private val gson = Gson()
    private val json = "application/json".toMediaType()
    private val http = OkHttpClient.Builder()
        .connectTimeout(timeoutSeconds, TimeUnit.SECONDS)
        .readTimeout(timeoutSeconds, TimeUnit.SECONDS)
        .build()

    @Volatile private var _accessToken = accessToken
    @Volatile private var _refreshToken = refreshToken
    @Volatile private var _tokenExpiry: Long = 0

    val accessToken: String get() = _accessToken

    fun setTokens(accessToken: String, refreshToken: String, expiresIn: Int) {
        _accessToken = accessToken
        _refreshToken = refreshToken
        _tokenExpiry = System.currentTimeMillis() + expiresIn * 1000L
    }

    fun clearTokens() {
        _accessToken = ""
        _refreshToken = ""
    }

    // HTTP 请求
    private inline fun <reified T> request(method: String, path: String, body: Any? = null, auth: Boolean = false): T {
        val url = "$baseUrl$path"
        val builder = Request.Builder().url(url)

        if (auth) {
            val token = ensureToken()
            builder.header("Authorization", "Bearer $token")
        }

        when (method) {
            "GET" -> builder.get()
            "POST" -> {
                val jsonBody = if (body != null) gson.toJson(body) else "{}"
                builder.post(jsonBody.toRequestBody(json))
            }
            "PUT" -> {
                val jsonBody = if (body != null) gson.toJson(body) else "{}"
                builder.put(jsonBody.toRequestBody(json))
            }
            "DELETE" -> {
                if (body != null) {
                    val jsonBody = gson.toJson(body)
                    builder.delete(jsonBody.toRequestBody(json))
                } else {
                    builder.delete()
                }
            }
            "PATCH" -> {
                val jsonBody = if (body != null) gson.toJson(body) else "{}"
                builder.patch(jsonBody.toRequestBody(json))
            }
            else -> throw IllegalArgumentException("Unsupported HTTP method: $method")
        }

        http.newCall(builder.build()).execute().use { resp ->
            val text = resp.body?.string() ?: ""
            if (!resp.isSuccessful) {
                throw SSOError.parse(resp.code, text)
            }
            if (text.isEmpty()) return MessageResponse() as T
            return gson.fromJson(text, object : TypeToken<T>() {}.type)
        }
    }

    private fun ensureToken(): String {
        if (_accessToken.isEmpty()) throw SSOError(401, "UNAUTHORIZED", "no access token")

        val needsRefresh = System.currentTimeMillis() > _tokenExpiry - 30_000
        if (needsRefresh && _refreshToken.isNotEmpty()) {
            val resp: TokenResponse = request("POST", "/api/v1/token",
                mapOf("grant_type" to "refresh_token", "refresh_token" to _refreshToken))
            setTokens(resp.accessToken, resp.refreshToken, resp.expiresIn)
            return resp.accessToken
        }
        return _accessToken
    }

    // 认证
    fun register(email: String, password: String): RegisterResponse =
        request("POST", "/api/v1/register", mapOf("email" to email, "password" to password))

    /**
     * 用户登录（第一阶段）
     *
     * 阶段 5.4 契约扩展：当用户启用 MFA 时，服务端返回 mfa_required=true 与
     * 一次性 mfa_challenge 令牌（TTL 5 分钟），此时 access_token/refresh_token 为空。
     * 本方法在这种情况下不会调用 setTokens；调用方应检查 resp.mfaRequired，
     * 若为 true 则提示用户输入 MFA 验证码并调用 verifyMFALogin 完成第二阶段登录。
     */
    fun login(email: String, password: String): TokenResponse {
        val resp: TokenResponse = request("POST", "/api/v1/login", mapOf("email" to email, "password" to password))
        if (resp.mfaRequired != true) {
            setTokens(resp.accessToken, resp.refreshToken, resp.expiresIn)
        }
        return resp
    }

    /**
     * MFA 两阶段登录第二阶段验证
     *
     * 阶段 5.4 契约扩展：POST /api/v1/login/mfa/verify
     * 使用 login 返回的 mfaChallenge 与用户输入的验证码完成登录。
     * 成功后服务端返回标准 TokenResponse，本方法会调用 setTokens 持久化。
     *
     * 失败错误码：MFA_CHALLENGE_INVALID / MFA_CHALLENGE_EXPIRED /
     * INVALID_MFA_CODE / TOO_MANY_MFA_ATTEMPTS / MFA_SERVICE_UNAVAILABLE
     */
    fun verifyMFALogin(req: LoginMFAVerifyRequest): TokenResponse {
        val resp: TokenResponse = request("POST", "/api/v1/login/mfa/verify", req)
        setTokens(resp.accessToken, resp.refreshToken, resp.expiresIn)
        return resp
    }

    fun refreshToken(): TokenResponse {
        if (_refreshToken.isEmpty()) throw SSOError(401, "UNAUTHORIZED", "no refresh token")
        val resp: TokenResponse = request("POST", "/api/v1/token",
            mapOf("grant_type" to "refresh_token", "refresh_token" to _refreshToken))
        setTokens(resp.accessToken, resp.refreshToken, resp.expiresIn)
        return resp
    }

    fun revokeToken(): MessageResponse {
        if (_accessToken.isEmpty()) return MessageResponse("no token")
        val resp: MessageResponse = request("POST", "/api/v1/token/revoke", mapOf("token" to _accessToken))
        clearTokens()
        return resp
    }

    fun forgotPassword(email: String): MessageResponse =
        request("POST", "/api/v1/forgot-password", mapOf("email" to email))

    fun resetPassword(token: String, userId: String, newPassword: String): MessageResponse =
        request("POST", "/api/v1/reset-password", mapOf("token" to token, "user_id" to userId, "new_password" to newPassword))

    // 用户
    fun userInfo(): UserInfo = request("GET", "/api/v1/userinfo", auth = true)

    fun changePassword(oldPassword: String, newPassword: String): MessageResponse =
        request("POST", "/api/v1/change-password", mapOf("old_password" to oldPassword, "new_password" to newPassword), auth = true)

    // MFA
    fun mfaSetup(): MFASetupResponse = request("POST", "/api/v1/mfa/setup", auth = true)
    fun mfaVerify(code: String): MessageResponse = request("POST", "/api/v1/mfa/verify", mapOf("code" to code), auth = true)
    fun mfaDisable(code: String): MessageResponse = request("POST", "/api/v1/mfa/disable", mapOf("code" to code), auth = true)
    fun mfaStatus(): MFAStatusResponse = request("GET", "/api/v1/mfa/status", auth = true)

    // 管理员
    fun adminHealth(): HealthResponse = request("GET", "/api/v1/admin/health", auth = true)
    fun listUsers(page: Int = 1, pageSize: Int = 20): UserListResponse =
        request("GET", "/api/v1/admin/users?page=$page&pageSize=$pageSize", auth = true)
    fun disableUser(userId: String): MessageResponse =
        request("POST", "/api/v1/admin/users/${encodePath(userId)}/disable", auth = true)
    fun enableUser(userId: String): MessageResponse =
        request("POST", "/api/v1/admin/users/${encodePath(userId)}/enable", auth = true)

    // 对路径参数做 URL 编码，避免特殊字符破坏路径
    private fun encodePath(value: String): String =
        URLEncoder.encode(value, "UTF-8")

    // ========================================================================
    // OAuth2
    // ========================================================================

    /**
     * 获取 OAuth2 授权（consent_token）
     *
     * 阶段 5.3 契约修复：服务端 GET /api/v1/authorize 返回 {consent_token, client_id,
     * redirect_uri, scope, state, require_approval}，不再直接返回 code。
     * 调用方应展示授权同意页面，用户同意后调用 [approveAuthorization] 获取 code。
     */
    fun authorize(clientId: String, redirectUri: String, scope: String, state: String): AuthorizeResponse {
        val params = mapOf(
            "client_id" to clientId, "redirect_uri" to redirectUri,
            "response_type" to "code", "scope" to scope, "state" to state
        )
        val query = params.entries.joinToString("&") { (k, v) ->
            "$k=${URLEncoder.encode(v, "UTF-8")}"
        }
        return request("GET", "/api/v1/authorize?$query", auth = true)
    }

    /**
     * 获取 OAuth2 授权（带 PKCE，consent_token）
     *
     * 阶段 5.3 新增：公共客户端必须使用 PKCE（S256）。
     */
    fun authorizeWithPKCE(
        clientId: String, redirectUri: String, scope: String, state: String, codeChallenge: String
    ): AuthorizeResponse {
        val params = mapOf(
            "client_id" to clientId, "redirect_uri" to redirectUri,
            "response_type" to "code", "scope" to scope, "state" to state,
            "code_challenge" to codeChallenge, "code_challenge_method" to "S256"
        )
        val query = params.entries.joinToString("&") { (k, v) ->
            "$k=${URLEncoder.encode(v, "UTF-8")}"
        }
        return request("GET", "/api/v1/authorize?$query", auth = true)
    }

    /**
     * 批准 OAuth2 授权
     *
     * 阶段 5.3 新增：服务端期望请求体 {consent_token, state}，
     * 不再接受 client_id/redirect_uri/scope 等字段（consent_token JWT 内部已携带）。
     * 调用方需先调用 [authorize]/[authorizeWithPKCE] 获取 consentToken，再传给本方法。
     *
     * 成功后返回 {code, state}，使用 code 调用 /api/v1/token 换取 Access Token。
     */
    fun approveAuthorization(req: AuthorizeApproveRequest): AuthorizeResponse =
        request("POST", "/api/v1/authorize/approve", req, auth = true)

    /**
     * 拒绝 OAuth2 授权
     *
     * 阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
     * 服务端固定返回 HTTP 403 + {error:"access_denied", error_description, state}，
     * 本方法将此响应当作正常的 DenyResponse 返回（不视为错误），
     * 调用方拿到后应向客户端应用回传 ?error=access_denied&state=xxx。
     *
     * 注意：仅在用户主动拒绝时调用；其他场景的 403 仍按错误处理。
     */
    fun denyAuthorization(req: AuthorizeDenyRequest): AuthorizeDenyResponse {
        return try {
            request("POST", "/api/v1/authorize/deny", req, auth = true)
        } catch (e: SSOError) {
            if (e.httpStatus == 403 && e.rawBody.isNotEmpty()) {
                try {
                    return gson.fromJson(e.rawBody, AuthorizeDenyResponse::class.java)
                } catch (_: Exception) {
                    // 解析失败则重新抛出原错误
                }
            }
            throw e
        }
    }

    // ========================================================================
    // Social Login 社交登录
    //
    // 阶段 5.5 新增：服务端契约
    //   - GET /auth/providers         公开端点，直接返回数组（不包裹 data）
    //   - GET /auth/{provider}?state= 公开端点，返回 HTTP 307 重定向到 provider 授权页面
    //   - GET /auth/{provider}/callback?code=&state= 公开端点，平铺返回 TokenResponse
    // ========================================================================

    /**
     * 获取支持的社交登录提供商列表
     *
     * 阶段 5.5 新增：调用 GET /auth/providers 公开端点。
     * 服务端直接返回数组（不包裹在 data 中），无需认证。
     */
    fun getProviders(): List<OAuthProvider> = request("GET", "/auth/providers")

    /**
     * 构造发起社交登录的 URL
     *
     * 阶段 5.5 新增：直接构造 URL 字符串，不发起 HTTP 请求。
     * 调用方应使用浏览器重定向到此 URL（服务端会返回 307 到 provider 授权页面），
     * 而不是 SDK 直接 GET。
     *
     * @param provider 社交登录提供商名称（如 "google" / "github"）
     * @param state    可选，CSRF 防护 state；为空时由服务端自动生成 UUID
     */
    fun getSocialLoginURL(provider: String, state: String = ""): String {
        val encodedProvider = URLEncoder.encode(provider, "UTF-8")
        val url = "$baseUrl/auth/$encodedProvider"
        return if (state.isNotEmpty()) {
            "$url?state=${URLEncoder.encode(state, "UTF-8")}"
        } else {
            url
        }
    }

    /**
     * 用回调返回的 code+state 完成社交登录
     *
     * 阶段 5.5 新增：调用 GET /auth/{provider}/callback?code={code}&state={state} 公开端点。
     * 服务端直接平铺返回 TokenResponse（不包裹 data），无需认证。
     * 成功后调用 setTokens 缓存到客户端。
     *
     * 失败错误码：MISSING_AUTH_CODE / OAUTH_STATE_INVALID / OAUTH_STATE_EXPIRED /
     * PROVIDER_NOT_SUPPORTED / OAUTH_CODE_EXCHANGE_FAILED / SOCIAL_LOGIN_FAILED /
     * PROVIDER_USER_ID_MISSING / PROVIDER_EMAIL_NOT_VERIFIED /
     * SOCIAL_ACCOUNT_CONFLICT / EMAIL_CONFLICT_WITH_LOCAL / ACCOUNT_DISABLED / ACCOUNT_LOCKED
     */
    fun exchangeSocialCode(provider: String, code: String, state: String): TokenResponse {
        val encodedProvider = URLEncoder.encode(provider, "UTF-8")
        val encodedCode = URLEncoder.encode(code, "UTF-8")
        val encodedState = URLEncoder.encode(state, "UTF-8")
        val path = "/auth/$encodedProvider/callback?code=$encodedCode&state=$encodedState"
        val resp: TokenResponse = request("GET", path)
        setTokens(resp.accessToken, resp.refreshToken, resp.expiresIn)
        return resp
    }

    // OIDC
    fun discovery(): DiscoveryResponse = request("GET", "/.well-known/openid-configuration")
    fun jwks(): JWKSResponse = request("GET", "/.well-known/jwks.json")
}
