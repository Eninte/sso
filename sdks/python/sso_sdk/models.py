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
    # 阶段 5.4 契约扩展：MFA 两阶段登录
    # 当 mfa_required 为 True 时，access_token/refresh_token 为空，
    # expires_in 表示 mfa_challenge 的 TTL（300 秒）。
    # 客户端需提示用户输入 MFA 验证码，再调用 verify_mfa_login 完成第二阶段登录。
    mfa_required: bool = False
    mfa_challenge: str = ""
    mfa_methods: list[str] = field(default_factory=list)


@dataclass
class LoginMFAVerifyRequest:
    """MFA 两阶段登录第二阶段验证请求

    阶段 5.4 契约扩展：当 login 返回 mfa_required=True 时，
    客户端使用返回的 mfa_challenge 调用本接口完成登录。

    - method: "totp"（6 位数字验证码）或 "recovery_code"（恢复码字符串）
    - code: 与 method 对应的验证值

    成功后服务端返回标准 TokenResponse（含 access_token/refresh_token）。
    """
    mfa_challenge: str = ""
    method: str = ""
    code: str = ""


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
    """授权响应

    阶段 5.3 契约修复：服务端 GET /api/v1/authorize 与 POST /api/v1/authorize/approve
    返回的响应结构不同。使用同一个 dataclass 兼容两种场景：
      - GET /authorize 返回：consent_token/client_id/redirect_uri/scope/state/require_approval
        （code 为空，前端需展示授权同意页面）
      - POST /authorize/approve 返回：code/state
        （consent_token 等字段为空，客户端使用 code 调用 /token 端点换取 Access Token）

    集成方应根据 require_approval 判断当前是处于"待同意"还是"已批准"状态。
    """
    # GET /authorize 返回字段
    consent_token: str = ""
    client_id: str = ""
    redirect_uri: str = ""
    scope: str = ""
    require_approval: bool = False

    # POST /authorize/approve 返回字段
    code: str = ""
    state: str = ""


@dataclass
class AuthorizeApproveRequest:
    """授权批准请求

    阶段 5.3 契约修复：服务端 /api/v1/authorize/approve 实际期望 {consent_token, state}。
    旧字段（client_id/redirect_uri/scope/code_challenge 等）已通过 consent_token JWT 携带，
    不再需要重复传递。请求体启用 DisallowUnknownFields，多余字段会被拒绝。
    """
    consent_token: str = ""
    state: str = ""


@dataclass
class AuthorizeDenyRequest:
    """授权拒绝请求

    阶段 5.3 新增：用户主动拒绝授权时调用 /api/v1/authorize/deny。
    """
    consent_token: str = ""
    state: str = ""


@dataclass
class AuthorizeDenyResponse:
    """授权拒绝响应

    阶段 5.3 新增：服务端返回 HTTP 403，error 固定为 "access_denied"。
    SDK 不应将其视为成功响应；调用方拿到此响应后应向客户端应用回传
    ?error=access_denied&state=xxx。
    """
    error: str = ""
    error_description: str = ""
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


@dataclass
class OAuthProvider:
    """OAuth 提供商

    阶段 5.5 新增：社交登录提供商信息，由 GET /auth/providers 返回。
    服务端直接返回数组（不包裹在 data 中）。
    """
    name: str = ""
    label: str = ""
    icon: str = ""
