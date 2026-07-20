"""SSO SDK 错误类型"""

from __future__ import annotations
from typing import Optional


class ErrorCode:
    """错误码常量"""

    INTERNAL = "INTERNAL_ERROR"
    BAD_REQUEST = "BAD_REQUEST"
    NOT_FOUND = "NOT_FOUND"
    CONFLICT = "CONFLICT"
    UNAUTHORIZED = "UNAUTHORIZED"
    FORBIDDEN = "FORBIDDEN"
    TOO_MANY_REQUESTS = "TOO_MANY_REQUESTS"
    INVALID_CREDENTIALS = "INVALID_CREDENTIALS"
    ACCOUNT_LOCKED = "ACCOUNT_LOCKED"
    ACCOUNT_DISABLED = "ACCOUNT_DISABLED"
    INVALID_TOKEN = "INVALID_TOKEN"
    TOKEN_EXPIRED = "TOKEN_EXPIRED"
    EMAIL_EXISTS = "EMAIL_EXISTS"
    EMAIL_INVALID = "EMAIL_INVALID"
    EMAIL_REQUIRED = "EMAIL_REQUIRED"
    PASSWORD_TOO_SHORT = "PASSWORD_TOO_SHORT"
    PASSWORD_TOO_LONG = "PASSWORD_TOO_LONG"
    PASSWORD_REQUIRED = "PASSWORD_REQUIRED"
    INVALID_REQUEST_FORMAT = "INVALID_REQUEST_FORMAT"
    REQUEST_BODY_TOO_LARGE = "REQUEST_BODY_TOO_LARGE"

    # === 阶段 5 SDK 同步：服务端阶段 2/3/4 引入的错误码 ===

    # Token 轮换 / 重放（阶段 2.1）
    # Refresh Token 已被使用过又再次出现，重放攻击典型特征
    # SDK 收到此错误应清空本地 Token 并要求用户重新登录
    TOKEN_ROTATED = "TOKEN_ROTATED"

    # OAuth Scope / PKCE / Consent（阶段 2.2）
    INVALID_SCOPE = "INVALID_SCOPE"            # scope 超出客户端允许或白名单
    PKCE_REQUIRED = "PKCE_REQUIRED"            # 公共客户端必须使用 PKCE（S256）
    CONSENT_REQUIRED = "CONSENT_REQUIRED"      # 需要用户同意授权
    CONSENT_DENIED = "CONSENT_DENIED"          # 用户拒绝授权
    CONSENT_INVALID = "CONSENT_INVALID"        # consent_token 无效或已过期
    CLIENT_MISMATCH = "CLIENT_MISMATCH"        # refresh_token 客户端归属不一致

    # MFA 两阶段登录（阶段 2.x）
    MFA_CHALLENGE_INVALID = "MFA_CHALLENGE_INVALID"      # Challenge 无效或已被使用
    MFA_CHALLENGE_EXPIRED = "MFA_CHALLENGE_EXPIRED"      # Challenge 已过期
    INVALID_MFA_CODE = "INVALID_MFA_CODE"                # TOTP 或恢复码无效
    TOO_MANY_MFA_ATTEMPTS = "TOO_MANY_MFA_ATTEMPTS"      # 尝试次数过多（默认 5 次）
    MFA_SERVICE_UNAVAILABLE = "MFA_SERVICE_UNAVAILABLE"  # MFA 服务未装配

    # Social Login 基础（阶段 2.2 改造）
    PROVIDER_NOT_SUPPORTED = "PROVIDER_NOT_SUPPORTED"
    OAUTH_CODE_EXCHANGE_FAILED = "OAUTH_CODE_EXCHANGE_FAILED"
    SOCIAL_LOGIN_FAILED = "SOCIAL_LOGIN_FAILED"
    OAUTH_STATE_INVALID = "OAUTH_STATE_INVALID"
    OAUTH_STATE_EXPIRED = "OAUTH_STATE_EXPIRED"

    # Social Login 安全增强（阶段 2.3 新增）
    PROVIDER_EMAIL_NOT_VERIFIED = "PROVIDER_EMAIL_NOT_VERIFIED"
    SOCIAL_ACCOUNT_CONFLICT = "SOCIAL_ACCOUNT_CONFLICT"
    EMAIL_CONFLICT_WITH_LOCAL = "EMAIL_CONFLICT_WITH_LOCAL"
    PROVIDER_USER_ID_MISSING = "PROVIDER_USER_ID_MISSING"

    # 邮件（阶段 4.3）
    # 服务端 SMTP 错误统一返回此通用错误码，不暴露 SMTP 内部信息
    EMAIL_SEND_FAILED = "EMAIL_SEND_FAILED"


class SSOError(Exception):
    """SSO API 错误"""

    def __init__(self, http_status: int, code: str, message: str = "", raw_body: str = ""):
        self.http_status = http_status
        self.code = code
        self.message = message
        self.raw_body = raw_body
        super().__init__(f"sso: {code} (HTTP {http_status}): {message}")

    def is_not_found(self) -> bool:
        return self.http_status == 404

    def is_unauthorized(self) -> bool:
        return self.http_status == 401

    def is_forbidden(self) -> bool:
        return self.http_status == 403

    def is_conflict(self) -> bool:
        return self.http_status == 409

    def is_rate_limited(self) -> bool:
        return self.http_status == 429

    # ========================================================================
    # 阶段 5.2 辅助判断方法
    #
    # 服务端阶段 2/3/4 引入了大量新的错误码，下游集成方若每次都通过字符串比较
    # 判断错误码可读性差且易错。这里提供一组语义化方法，覆盖最常见的安全处理分支。
    # ========================================================================

    def is_token_rotated(self) -> bool:
        """Refresh Token 已被使用过（重放攻击特征）

        收到此错误应立即清空本地 access_token 和 refresh_token，
        并要求用户重新登录。服务端会同步撤销该用户的所有 Token。
        """
        return self.code == ErrorCode.TOKEN_ROTATED

    def is_consent_required(self) -> bool:
        """需要用户同意授权（CONSENT_REQUIRED 或 CONSENT_INVALID）

        收到此错误应重新调用 authorize 获取 consent_token，
        并展示授权同意页面。
        """
        return self.code in (ErrorCode.CONSENT_REQUIRED, ErrorCode.CONSENT_INVALID)

    def is_consent_denied(self) -> bool:
        """用户主动拒绝授权，应终止授权流程"""
        return self.code == ErrorCode.CONSENT_DENIED

    def is_pkce_required(self) -> bool:
        """公共客户端必须使用 PKCE（S256），应生成 code_verifier 重新发起授权"""
        return self.code == ErrorCode.PKCE_REQUIRED

    def is_invalid_scope(self) -> bool:
        """请求的 scope 超出客户端允许范围或不在白名单"""
        return self.code == ErrorCode.INVALID_SCOPE

    def is_client_mismatch(self) -> bool:
        """Refresh Token 与客户端归属不一致"""
        return self.code == ErrorCode.CLIENT_MISMATCH

    def is_mfa_challenge_invalid(self) -> bool:
        """MFA Challenge 无效或已被使用，应重新触发登录"""
        return self.code == ErrorCode.MFA_CHALLENGE_INVALID

    def is_mfa_challenge_expired(self) -> bool:
        """MFA Challenge 已过期，应重新触发登录"""
        return self.code == ErrorCode.MFA_CHALLENGE_EXPIRED

    def is_too_many_mfa_attempts(self) -> bool:
        """MFA 验证尝试次数过多（默认 5 次），challenge 已失效"""
        return self.code == ErrorCode.TOO_MANY_MFA_ATTEMPTS

    def is_social_login_error(self) -> bool:
        """社交登录相关错误（统一处理）

        涵盖阶段 2.2/2.3 引入的所有社交登录错误码。
        """
        return self.code in (
            ErrorCode.PROVIDER_NOT_SUPPORTED,
            ErrorCode.OAUTH_CODE_EXCHANGE_FAILED,
            ErrorCode.SOCIAL_LOGIN_FAILED,
            ErrorCode.OAUTH_STATE_INVALID,
            ErrorCode.OAUTH_STATE_EXPIRED,
            ErrorCode.PROVIDER_EMAIL_NOT_VERIFIED,
            ErrorCode.SOCIAL_ACCOUNT_CONFLICT,
            ErrorCode.EMAIL_CONFLICT_WITH_LOCAL,
            ErrorCode.PROVIDER_USER_ID_MISSING,
        )

    def is_email_send_failed(self) -> bool:
        """邮件发送失败（SMTP 错误统一返回此码，不暴露内部信息）"""
        return self.code == ErrorCode.EMAIL_SEND_FAILED


def parse_error(http_status: int, body: str) -> SSOError:
    """解析错误响应体"""
    import json

    code = ""
    message = ""
    try:
        parsed = json.loads(body)
        code = parsed.get("code") or parsed.get("error", "")
        message = parsed.get("message", "")
    except (json.JSONDecodeError, TypeError):
        pass

    return SSOError(http_status, code, message or body, body)
