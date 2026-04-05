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
