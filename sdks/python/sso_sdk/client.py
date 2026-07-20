"""SSO Service Python Client"""

from __future__ import annotations

import json
import time
import urllib.parse
import urllib.request
import urllib.error
from typing import Any, Optional
from threading import Lock

from .errors import SSOError, parse_error
from .models import (
    TokenResponse, LoginMFAVerifyRequest, RegisterResponse, UserInfo, MessageResponse,
    MFASetupResponse, MFAStatusResponse, AuthorizeResponse,
    AuthorizeApproveRequest, AuthorizeDenyRequest, AuthorizeDenyResponse,
    UserListResponse, UserItem, HealthResponse, DiscoveryResponse, JWKSResponse, JWK,
    OAuthProvider,
)


class SSOClient:
    """SSO 服务客户端"""

    def __init__(
        self,
        base_url: str,
        *,
        access_token: str = "",
        refresh_token: str = "",
        timeout: int = 30,
    ):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self._access_token = access_token
        self._refresh_token = refresh_token
        self._token_expiry: float = 0
        self._lock = Lock()

    @property
    def access_token(self) -> str:
        return self._access_token

    def set_tokens(self, access_token: str, refresh_token: str, expires_in: int) -> None:
        with self._lock:
            self._access_token = access_token
            self._refresh_token = refresh_token
            self._token_expiry = time.time() + expires_in

    def clear_tokens(self) -> None:
        with self._lock:
            self._access_token = ""
            self._refresh_token = ""
            self._token_expiry = 0

    # =======================================================================
    # HTTP 请求
    # =======================================================================

    def _request(
        self,
        method: str,
        path: str,
        body: Optional[dict] = None,
        auth: bool = False,
    ) -> Any:
        url = f"{self.base_url}{path}"
        headers = {"Content-Type": "application/json", "Accept": "application/json"}

        if auth:
            token = self._ensure_token()
            headers["Authorization"] = f"Bearer {token}"

        data = json.dumps(body).encode("utf-8") if body else None

        req = urllib.request.Request(url, data=data, headers=headers, method=method)

        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                text = resp.read().decode("utf-8")
                return json.loads(text) if text else {}
        except urllib.error.HTTPError as e:
            body_text = e.read().decode("utf-8", errors="replace")
            raise parse_error(e.code, body_text) from e
        except urllib.error.URLError as e:
            raise SSOError(0, "CONNECTION_ERROR", str(e.reason)) from e

    def _ensure_token(self) -> str:
        if not self._access_token:
            raise SSOError(401, "UNAUTHORIZED", "no access token available, please login first")

        if time.time() > self._token_expiry - 30 and self._refresh_token:
            resp = self._request("POST", "/api/v1/token", {
                "grant_type": "refresh_token",
                "refresh_token": self._refresh_token,
            })
            self.set_tokens(resp["access_token"], resp["refresh_token"], resp["expires_in"])
            return resp["access_token"]

        return self._access_token

    # =======================================================================
    # 认证
    # =======================================================================

    def register(self, email: str, password: str) -> RegisterResponse:
        resp = self._request("POST", "/api/v1/register", {"email": email, "password": password})
        # 服务端注册响应只返回 message（防用户枚举），不返回 data
        return RegisterResponse(message=resp.get("message", ""))

    def login(self, email: str, password: str) -> TokenResponse:
        """用户登录（第一阶段）

        阶段 5.4 契约扩展：当用户启用 MFA 时，服务端返回 mfa_required=True 与
        一次性 mfa_challenge 令牌（TTL 5 分钟），此时 access_token/refresh_token 为空。
        本方法在这种情况下不会调用 set_tokens；调用方应检查 resp.mfa_required，
        若为 True 则提示用户输入 MFA 验证码并调用 verify_mfa_login 完成第二阶段登录。
        """
        resp = self._request("POST", "/api/v1/login", {"email": email, "password": password})
        token_resp = self._to_token_response(resp)
        if not token_resp.mfa_required:
            self.set_tokens(token_resp.access_token, token_resp.refresh_token, token_resp.expires_in)
        return token_resp

    def verify_mfa_login(self, req: LoginMFAVerifyRequest) -> TokenResponse:
        """MFA 两阶段登录第二阶段验证

        阶段 5.4 契约扩展：POST /api/v1/login/mfa/verify
        使用 login 返回的 mfa_challenge 与用户输入的验证码完成登录。
        成功后服务端返回标准 TokenResponse，本方法会调用 set_tokens 持久化。

        失败错误码：MFA_CHALLENGE_INVALID / MFA_CHALLENGE_EXPIRED /
        INVALID_MFA_CODE / TOO_MANY_MFA_ATTEMPTS / MFA_SERVICE_UNAVAILABLE
        """
        body = {
            "mfa_challenge": req.mfa_challenge,
            "method": req.method,
            "code": req.code,
        }
        resp = self._request("POST", "/api/v1/login/mfa/verify", body)
        token_resp = self._to_token_response(resp)
        self.set_tokens(token_resp.access_token, token_resp.refresh_token, token_resp.expires_in)
        return token_resp

    def refresh_token(self) -> TokenResponse:
        if not self._refresh_token:
            raise SSOError(401, "UNAUTHORIZED", "no refresh token available")
        resp = self._request("POST", "/api/v1/token", {
            "grant_type": "refresh_token",
            "refresh_token": self._refresh_token,
        })
        self.set_tokens(resp["access_token"], resp["refresh_token"], resp["expires_in"])
        return self._to_token_response(resp)

    def exchange_code(
        self,
        code: str,
        client_id: str,
        client_secret: str,
        redirect_uri: str,
        code_verifier: str = "",
    ) -> TokenResponse:
        body = {
            "grant_type": "authorization_code",
            "code": code,
            "client_id": client_id,
            "client_secret": client_secret,
            "redirect_uri": redirect_uri,
        }
        if code_verifier:
            body["code_verifier"] = code_verifier
        resp = self._request("POST", "/api/v1/token", body)
        # 服务端 authorization_code 响应包了一层 data，需要剥离后读取 token
        data = resp.get("data", resp)
        self.set_tokens(data["access_token"], data["refresh_token"], data["expires_in"])
        return self._to_token_response(data)

    def revoke_token(self) -> MessageResponse:
        if not self._access_token:
            return MessageResponse(message="no token to revoke")
        resp = self._request("POST", "/api/v1/token/revoke", {"token": self._access_token})
        self.clear_tokens()
        return MessageResponse(message=resp.get("message", ""))

    def forgot_password(self, email: str) -> MessageResponse:
        resp = self._request("POST", "/api/v1/forgot-password", {"email": email})
        return MessageResponse(message=resp.get("message", ""))

    def reset_password(self, token: str, user_id: str, new_password: str) -> MessageResponse:
        resp = self._request("POST", "/api/v1/reset-password", {
            "token": token, "user_id": user_id, "new_password": new_password,
        })
        return MessageResponse(message=resp.get("message", ""))

    def verify_email(self, token: str, user_id: str) -> MessageResponse:
        params = urllib.parse.urlencode({"token": token, "user_id": user_id})
        resp = self._request("GET", f"/api/v1/verify-email?{params}")
        return MessageResponse(message=resp.get("message", ""))

    def send_verification_email(self) -> MessageResponse:
        resp = self._request("POST", "/api/v1/verify-email/send", auth=True)
        return MessageResponse(message=resp.get("message", ""))

    # =======================================================================
    # 用户
    # =======================================================================

    def user_info(self) -> UserInfo:
        resp = self._request("GET", "/api/v1/userinfo", auth=True)
        return UserInfo(
            sub=resp.get("sub", ""),
            email=resp.get("email", ""),
            email_verified=resp.get("email_verified", False),
            scope=resp.get("scope", []),
        )

    def change_password(self, old_password: str, new_password: str) -> MessageResponse:
        resp = self._request("POST", "/api/v1/change-password", {
            "old_password": old_password, "new_password": new_password,
        }, auth=True)
        return MessageResponse(message=resp.get("message", ""))

    # =======================================================================
    # OAuth2
    # =======================================================================

    def authorize(self, client_id: str, redirect_uri: str, scope: str, state: str) -> AuthorizeResponse:
        """获取 OAuth2 授权（consent_token）

        阶段 5.3 契约修复：服务端 GET /api/v1/authorize 返回 {consent_token, client_id,
        redirect_uri, scope, state, require_approval}，不再直接返回 code。
        调用方应展示授权同意页面，用户同意后调用 approve_authorization 获取 code。
        """
        params = urllib.parse.urlencode({
            "client_id": client_id, "redirect_uri": redirect_uri,
            "response_type": "code", "scope": scope, "state": state,
        })
        resp = self._request("GET", f"/api/v1/authorize?{params}", auth=True)
        return self._to_authorize_response(resp)

    def authorize_with_pkce(
        self, client_id: str, redirect_uri: str, scope: str, state: str, code_challenge: str,
    ) -> AuthorizeResponse:
        """获取 OAuth2 授权（带 PKCE，consent_token）

        阶段 5.3 契约修复：同 authorize()，但携带 PKCE challenge。
        公共客户端必须使用此方法（S256）。
        """
        params = urllib.parse.urlencode({
            "client_id": client_id, "redirect_uri": redirect_uri,
            "response_type": "code", "scope": scope, "state": state,
            "code_challenge": code_challenge, "code_challenge_method": "S256",
        })
        resp = self._request("GET", f"/api/v1/authorize?{params}", auth=True)
        return self._to_authorize_response(resp)

    def approve_authorization(self, req: AuthorizeApproveRequest) -> AuthorizeResponse:
        """批准 OAuth2 授权

        阶段 5.3 契约修复：服务端期望请求体 {consent_token, state}，
        不再接受 client_id/redirect_uri/scope 等字段（consent_token JWT 内部已携带）。
        调用方需先调用 authorize/authorize_with_pkce 获取 consent_token，再传给本方法。

        成功后返回 {code, state}，使用 code 调用 exchange_code 换取 Access Token。
        """
        body = {"consent_token": req.consent_token, "state": req.state}
        resp = self._request("POST", "/api/v1/authorize/approve", body, auth=True)
        return self._to_authorize_response(resp)

    def deny_authorization(self, req: AuthorizeDenyRequest) -> AuthorizeDenyResponse:
        """拒绝 OAuth2 授权

        阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
        服务端固定返回 HTTP 403 + {error:"access_denied", error_description, state}，
        本方法将此响应当作正常的 DenyResponse 返回（不视为错误），
        调用方拿到后应向客户端应用回传 ?error=access_denied&state=xxx。

        注意：仅在用户主动拒绝时调用；其他场景的 403 仍按错误处理。
        """
        body = {"consent_token": req.consent_token, "state": req.state}
        try:
            resp = self._request("POST", "/api/v1/authorize/deny", body, auth=True)
            # 理论上不会进入此分支（服务端固定返回 403），但保留兼容性
            return AuthorizeDenyResponse(
                error=resp.get("error", ""),
                error_description=resp.get("error_description", ""),
                state=resp.get("state", ""),
            )
        except SSOError as e:
            if e.http_status == 403:
                import json as _json
                try:
                    resp = _json.loads(e.raw_body)
                    return AuthorizeDenyResponse(
                        error=resp.get("error", ""),
                        error_description=resp.get("error_description", ""),
                        state=resp.get("state", ""),
                    )
                except (ValueError, TypeError):
                    pass
            raise

    @staticmethod
    def _to_authorize_response(resp: Any) -> AuthorizeResponse:
        """从 dict 构造 AuthorizeResponse（兼容 GET 与 POST approve 两种响应）"""
        return AuthorizeResponse(
            consent_token=resp.get("consent_token", ""),
            client_id=resp.get("client_id", ""),
            redirect_uri=resp.get("redirect_uri", ""),
            scope=resp.get("scope", ""),
            require_approval=resp.get("require_approval", False),
            code=resp.get("code", ""),
            state=resp.get("state", ""),
        )

    # =======================================================================
    # Social Login 社交登录
    #
    # 阶段 5.5 新增：服务端契约
    #   - GET /auth/providers         公开端点，直接返回数组（不包裹 data）
    #   - GET /auth/{provider}?state= 公开端点，返回 HTTP 307 重定向到 provider 授权页面
    #   - GET /auth/{provider}/callback?code=&state= 公开端点，平铺返回 TokenResponse
    # =======================================================================

    def get_providers(self) -> list[OAuthProvider]:
        """获取支持的社交登录提供商列表

        阶段 5.5 新增：调用 GET /auth/providers 公开端点。
        服务端直接返回数组（不包裹在 data 中），无需认证。
        """
        resp = self._request("GET", "/auth/providers")
        return [
            OAuthProvider(
                name=item.get("name", ""),
                label=item.get("label", ""),
                icon=item.get("icon", ""),
            )
            for item in resp
        ]

    def get_social_login_url(self, provider: str, state: str = "") -> str:
        """构造发起社交登录的 URL

        阶段 5.5 新增：直接构造 URL 字符串，不发起 HTTP 请求。
        调用方应使用浏览器重定向到此 URL（服务端会返回 307 到 provider 授权页面），
        而不是 SDK 直接 GET。

        :param provider: 社交登录提供商名称（如 "google" / "github"）
        :param state:    可选，CSRF 防护 state；为空时由服务端自动生成 UUID
        """
        encoded_provider = urllib.parse.quote(provider, safe="")
        url = f"{self.base_url}/auth/{encoded_provider}"
        if state:
            url += f"?{urllib.parse.urlencode({'state': state})}"
        return url

    def exchange_social_code(self, provider: str, code: str, state: str) -> TokenResponse:
        """用回调返回的 code+state 完成社交登录

        阶段 5.5 新增：调用 GET /auth/{provider}/callback?code={code}&state={state} 公开端点。
        服务端直接平铺返回 TokenResponse（不包裹 data），无需认证。
        成功后调用 set_tokens 缓存到客户端。

        失败错误码：MISSING_AUTH_CODE / OAUTH_STATE_INVALID / OAUTH_STATE_EXPIRED /
        PROVIDER_NOT_SUPPORTED / OAUTH_CODE_EXCHANGE_FAILED / SOCIAL_LOGIN_FAILED /
        PROVIDER_USER_ID_MISSING / PROVIDER_EMAIL_NOT_VERIFIED /
        SOCIAL_ACCOUNT_CONFLICT / EMAIL_CONFLICT_WITH_LOCAL / ACCOUNT_DISABLED / ACCOUNT_LOCKED
        """
        encoded_provider = urllib.parse.quote(provider, safe="")
        params = urllib.parse.urlencode({"code": code, "state": state})
        resp = self._request("GET", f"/auth/{encoded_provider}/callback?{params}")
        token_resp = self._to_token_response(resp)
        self.set_tokens(token_resp.access_token, token_resp.refresh_token, token_resp.expires_in)
        return token_resp

    # =======================================================================
    # MFA
    # =======================================================================

    def mfa_setup(self) -> MFASetupResponse:
        resp = self._request("POST", "/api/v1/mfa/setup", auth=True)
        return MFASetupResponse(
            secret=resp.get("secret", ""),
            qr_code_url=resp.get("qr_code_url", ""),
            manual_entry=resp.get("manual_entry", ""),
        )

    def mfa_verify(self, code: str) -> MessageResponse:
        resp = self._request("POST", "/api/v1/mfa/verify", {"code": code}, auth=True)
        return MessageResponse(message=resp.get("message", ""))

    def mfa_disable(self, code: str) -> MessageResponse:
        resp = self._request("POST", "/api/v1/mfa/disable", {"code": code}, auth=True)
        return MessageResponse(message=resp.get("message", ""))

    def mfa_status(self) -> MFAStatusResponse:
        resp = self._request("GET", "/api/v1/mfa/status", auth=True)
        return MFAStatusResponse(enabled=resp.get("enabled", False))

    # =======================================================================
    # 管理员
    # =======================================================================

    def admin_health(self) -> HealthResponse:
        resp = self._request("GET", "/api/v1/admin/health", auth=True)
        return HealthResponse(
            status=resp.get("status", ""),
            timestamp=resp.get("timestamp", ""),
            database=resp.get("database", ""),
            version=resp.get("version", ""),
        )

    def admin_cleanup(self) -> MessageResponse:
        resp = self._request("POST", "/api/v1/admin/cleanup", auth=True)
        return MessageResponse(message=resp.get("message", ""))

    def list_users(self, page: int = 1, page_size: int = 20) -> UserListResponse:
        resp = self._request("GET", f"/api/v1/admin/users?page={page}&pageSize={page_size}", auth=True)
        users = [UserItem(**u) for u in resp.get("users", [])]
        return UserListResponse(
            users=users, total=resp.get("total", 0),
            page=resp.get("page", 1), page_size=resp.get("page_size", 20),
            total_pages=resp.get("total_pages", 0),
        )

    def get_user(self, user_id: str) -> UserItem:
        # 使用路径参数，用 urllib.parse.quote 编码避免特殊字符问题
        encoded_id = urllib.parse.quote(str(user_id))
        resp = self._request("GET", f"/api/v1/admin/users/{encoded_id}", auth=True)
        data = {}
        for k, field in UserItem.__dataclass_fields__.items():
            default = field.default if field.default is not field.default_factory else ""
            data[k] = resp.get(k, default)
        return UserItem(**data)

    def disable_user(self, user_id: str) -> MessageResponse:
        # 使用路径参数，不再通过 body 传递 user_id
        encoded_id = urllib.parse.quote(str(user_id))
        resp = self._request("POST", f"/api/v1/admin/users/{encoded_id}/disable", auth=True)
        return MessageResponse(message=resp.get("message", ""))

    def enable_user(self, user_id: str) -> MessageResponse:
        # 使用路径参数，不再通过 body 传递 user_id
        encoded_id = urllib.parse.quote(str(user_id))
        resp = self._request("POST", f"/api/v1/admin/users/{encoded_id}/enable", auth=True)
        return MessageResponse(message=resp.get("message", ""))

    # =======================================================================
    # OIDC
    # =======================================================================

    def discovery(self) -> DiscoveryResponse:
        resp = self._request("GET", "/.well-known/openid-configuration")
        return DiscoveryResponse(
            issuer=resp.get("issuer", ""),
            authorization_endpoint=resp.get("authorization_endpoint", ""),
            token_endpoint=resp.get("token_endpoint", ""),
            userinfo_endpoint=resp.get("userinfo_endpoint", ""),
            jwks_uri=resp.get("jwks_uri", ""),
            revocation_endpoint=resp.get("revocation_endpoint", ""),
            grant_types_supported=resp.get("grant_types_supported", []),
            code_challenge_methods_supported=resp.get("code_challenge_methods_supported", []),
        )

    def jwks(self) -> JWKSResponse:
        resp = self._request("GET", "/.well-known/jwks.json")
        keys = [JWK(**k) for k in resp.get("keys", [])]
        return JWKSResponse(keys=keys)

    # =======================================================================
    # 辅助方法
    # =======================================================================

    @staticmethod
    def _to_token_response(data: dict) -> TokenResponse:
        return TokenResponse(
            access_token=data.get("access_token", ""),
            refresh_token=data.get("refresh_token", ""),
            token_type=data.get("token_type", "Bearer"),
            expires_in=data.get("expires_in", 0),
            scopes=data.get("scopes", []),
            scope=data.get("scope", ""),
            mfa_required=data.get("mfa_required", False),
            mfa_challenge=data.get("mfa_challenge", ""),
            mfa_methods=data.get("mfa_methods", []),
        )
