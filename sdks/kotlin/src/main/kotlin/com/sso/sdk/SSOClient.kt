package com.sso.sdk

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import com.google.gson.reflect.TypeToken
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
}

// ============================================================================
// 响应模型
// ============================================================================

data class TokenResponse(
    @SerializedName("access_token") val accessToken: String,
    @SerializedName("refresh_token") val refreshToken: String,
    @SerializedName("token_type") val tokenType: String = "Bearer",
    @SerializedName("expires_in") val expiresIn: Int = 0
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

    fun login(email: String, password: String): TokenResponse {
        val resp: TokenResponse = request("POST", "/api/v1/login", mapOf("email" to email, "password" to password))
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
    fun adminHealth(): HealthResponse = request("GET", "/admin/health", auth = true)
    fun listUsers(page: Int = 1, pageSize: Int = 20): UserListResponse =
        request("GET", "/admin/users?page=$page&pageSize=$pageSize", auth = true)
    fun disableUser(userId: String): MessageResponse =
        request("POST", "/admin/users/disable", mapOf("user_id" to userId), auth = true)
    fun enableUser(userId: String): MessageResponse =
        request("POST", "/admin/users/enable", mapOf("user_id" to userId), auth = true)

    // OIDC
    fun discovery(): DiscoveryResponse = request("GET", "/.well-known/openid-configuration")
    fun jwks(): JWKSResponse = request("GET", "/.well-known/jwks.json")
}
