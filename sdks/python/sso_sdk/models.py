"""SSO SDK 数据模型"""

from __future__ import annotations
from dataclasses import dataclass, field


@dataclass
class TokenResponse:
    access_token: str = ""
    refresh_token: str = ""
    token_type: str = "Bearer"
    expires_in: int = 0
    scopes: list[str] = field(default_factory=list)
    scope: str = ""


@dataclass
class RegisterResponse:
    # 服务端注册响应只有 message 字段（防用户枚举），不返回 data
    message: str = ""


@dataclass
class UserInfo:
    sub: str = ""
    email: str = ""
    email_verified: bool = False
    # 服务端返回 scope 单数，类型为 list
    scope: list[str] = field(default_factory=list)


@dataclass
class MessageResponse:
    message: str = ""


@dataclass
class MFASetupResponse:
    secret: str = ""
    qr_code_url: str = ""
    manual_entry: str = ""


@dataclass
class MFAStatusResponse:
    enabled: bool = False


@dataclass
class AuthorizeResponse:
    code: str = ""
    state: str = ""


@dataclass
class UserItem:
    id: str = ""
    email: str = ""
    email_verified: bool = False
    mfa_enabled: bool = False
    status: str = ""
    created_at: str = ""
    updated_at: str = ""


@dataclass
class UserListResponse:
    users: list[UserItem] = field(default_factory=list)
    total: int = 0
    page: int = 1
    page_size: int = 20
    total_pages: int = 0


@dataclass
class HealthResponse:
    status: str = ""
    timestamp: str = ""
    database: str = ""
    version: str = ""


@dataclass
class DiscoveryResponse:
    issuer: str = ""
    authorization_endpoint: str = ""
    token_endpoint: str = ""
    userinfo_endpoint: str = ""
    jwks_uri: str = ""
    revocation_endpoint: str = ""
    grant_types_supported: list[str] = field(default_factory=list)
    code_challenge_methods_supported: list[str] = field(default_factory=list)


@dataclass
class JWK:
    kty: str = ""
    use: str = ""
    kid: str = ""
    n: str = ""
    e: str = ""


@dataclass
class JWKSResponse:
    keys: list[JWK] = field(default_factory=list)
