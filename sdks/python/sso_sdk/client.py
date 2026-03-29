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
    TokenResponse, RegisterResponse, RegisterData, UserInfo, MessageResponse,
    MFASetupResponse, MFAStatusResponse, AuthorizeResponse,
    UserListResponse, UserItem, HealthResponse, DiscoveryResponse, JWKSResponse, JWK,
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
        resp = self._request("POST", "/api/v1/register", {"email": password})
        data = resp.get("data", {})
        return RegisterResponse(
            message=resp.get("message", ""),
            data=RegisterData(user_id=data.get("user_id", ""), email=data.get("email", "")) if data else None,
        )

    def login(self, email: str, password: str) -> TokenResponse:
        resp = self._request("POST", "/api/v1/login", {"email": email, "password": password})
        self.set_tokens(resp["access_token"], resp["refresh_token"], resp["expires_in"])
        return self._to_token_response(resp)

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
        return self._to_token_response(resp)

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
            scopes=resp.get("scopes", []),
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
        params = urllib.parse.urlencode({
            "client_id": client_id, "redirect_uri": redirect_uri,
            "response_type": "code", "scope": scope, "state": state,
        })
        resp = self._request("GET", f"/api/v1/authorize?{params}", auth=True)
        return AuthorizeResponse(code=resp.get("code", ""), state=resp.get("state", ""))

    def authorize_with_pkce(
        self, client_id: str, redirect_uri: str, scope: str, state: str, code_challenge: str,
    ) -> AuthorizeResponse:
        params = urllib.parse.urlencode({
            "client_id": client_id, "redirect_uri": redirect_uri,
            "response_type": "code", "scope": scope, "state": state,
            "code_challenge": code_challenge, "code_challenge_method": "S256",
        })
        resp = self._request("GET", f"/api/v1/authorize?{params}", auth=True)
        return AuthorizeResponse(code=resp.get("code", ""), state=resp.get("state", ""))

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
        resp = self._request("GET", "/admin/health", auth=True)
        return HealthResponse(
            status=resp.get("status", ""),
            timestamp=resp.get("timestamp", ""),
            database=resp.get("database", ""),
            version=resp.get("version", ""),
        )

    def admin_cleanup(self) -> MessageResponse:
        resp = self._request("POST", "/admin/cleanup", auth=True)
        return MessageResponse(message=resp.get("message", ""))

    def list_users(self, page: int = 1, page_size: int = 20) -> UserListResponse:
        resp = self._request("GET", f"/admin/users?page={page}&pageSize={page_size}", auth=True)
        users = [UserItem(**u) for u in resp.get("users", [])]
        return UserListResponse(
            users=users, total=resp.get("total", 0),
            page=resp.get("page", 1), page_size=resp.get("page_size", 20),
            total_pages=resp.get("total_pages", 0),
        )

    def get_user(self, user_id: str) -> UserItem:
        resp = self._request("GET", f"/admin/users?id={user_id}", auth=True)
        return UserItem(**{k: resp.get(k, "") for k in UserItem.__dataclass_fields__})

    def disable_user(self, user_id: str) -> MessageResponse:
        resp = self._request("POST", "/admin/users/disable", {"user_id": user_id}, auth=True)
        return MessageResponse(message=resp.get("message", ""))

    def enable_user(self, user_id: str) -> MessageResponse:
        resp = self._request("POST", "/admin/users/enable", {"user_id": user_id}, auth=True)
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
        )
