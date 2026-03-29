"""SSO Service Python Client SDK"""

__version__ = "1.0.0"

from .client import SSOClient
from .errors import SSOError, ErrorCode

__all__ = ["SSOClient", "SSOError", "ErrorCode"]
