"""SSO SDK 单元测试"""

import json
import sys
import os
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from sso_sdk import SSOClient, SSOError


def run_mock_server(handler_map):
    """启动 mock 服务器，返回 (url, server)
    handler_map: {"GET:/path": lambda body: (status, dict), ...}
    """
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self): self._handle("GET")
        def do_POST(self): self._handle("POST")

        def _handle(self, method):
            body = ""
            if self.headers.get("Content-Length"):
                body = self.rfile.read(int(self.headers["Content-Length"])).decode()
            key = f"{method}:{self.path.split('?')[0]}"
            fn = handler_map.get(key)
            if not fn:
                self.send_response(404)
                self.end_headers()
                return
            status, resp = fn(body)
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(resp).encode())
        def log_message(self, *args): pass

    server = HTTPServer(("127.0.0.1", 0), Handler)
    port = server.server_address[1]
    threading.Thread(target=server.serve_forever, daemon=True).start()
    time.sleep(0.05)
    return f"http://127.0.0.1:{port}", server


def test_create_client():
    c = SSOClient("http://localhost:9090")
    assert c.base_url == "http://localhost:9090"
    assert c.access_token == ""

def test_trailing_slash():
    assert SSOClient("http://x/").base_url == "http://x"

def test_set_clear_tokens():
    c = SSOClient("http://x")
    c.set_tokens("a", "b", 900)
    assert c.access_token == "a"
    c.clear_tokens()
    assert c.access_token == ""

def test_register():
    # 服务端注册响应只有 message 字段（防用户枚举），不返回 data
    url, svr = run_mock_server({
        "POST:/api/v1/register": lambda _: (201, {"message": "ok"}),
    })
    try:
        r = SSOClient(url).register("t@e.com", "P@ss1")
        assert r.message == "ok"
    finally:
        svr.shutdown()

def test_register_conflict():
    url, svr = run_mock_server({
        "POST:/api/v1/register": lambda _: (409, {"code": "EMAIL_EXISTS", "message": "exists"}),
    })
    try:
        try:
            SSOClient(url).register("x@e.com", "P@ss1")
            assert False
        except SSOError as e:
            assert e.is_conflict()
            assert e.code == "EMAIL_EXISTS"
    finally:
        svr.shutdown()

def test_login():
    url, svr = run_mock_server({
        "POST:/api/v1/login": lambda _: (200, {
            "access_token": "a1", "refresh_token": "r1", "token_type": "Bearer", "expires_in": 900,
        }),
    })
    try:
        c = SSOClient(url)
        r = c.login("t@e.com", "P@ss1")
        assert r.access_token == "a1"
        assert c.access_token == "a1"
    finally:
        svr.shutdown()

def test_login_fail():
    url, svr = run_mock_server({
        "POST:/api/v1/login": lambda _: (401, {"code": "INVALID_CREDENTIALS", "message": "wrong"}),
    })
    try:
        try:
            SSOClient(url).login("t@e.com", "wrong")
            assert False
        except SSOError as e:
            assert e.is_unauthorized()
    finally:
        svr.shutdown()

def test_user_info():
    # 服务端返回 scope 单数，类型为 list
    url, svr = run_mock_server({
        "GET:/api/v1/userinfo": lambda _: (200, {
            "sub": "u1", "email": "t@e.com", "email_verified": True, "scope": ["read", "write"],
        }),
    })
    try:
        r = SSOClient(url, access_token="tok").user_info()
        assert r.sub == "u1"
        assert r.scope == ["read", "write"]
    finally:
        svr.shutdown()

def test_user_info_no_token():
    try:
        SSOClient("http://x").user_info()
        assert False
    except SSOError as e:
        assert e.is_unauthorized()

def test_revoke():
    url, svr = run_mock_server({
        "POST:/api/v1/token/revoke": lambda _: (200, {"message": "ok"}),
    })
    try:
        c = SSOClient(url, access_token="tok")
        c.revoke_token()
        assert c.access_token == ""
    finally:
        svr.shutdown()

def test_forgot_password():
    url, svr = run_mock_server({
        "POST:/api/v1/forgot-password": lambda _: (200, {"message": "sent"}),
    })
    try:
        r = SSOClient(url).forgot_password("t@e.com")
        assert r.message == "sent"
    finally:
        svr.shutdown()

def test_mfa_setup():
    url, svr = run_mock_server({
        "POST:/api/v1/mfa/setup": lambda _: (200, {"secret": "ABC", "qr_code_url": "otpauth://", "manual_entry": "ABC"}),
    })
    try:
        r = SSOClient(url, access_token="t").mfa_setup()
        assert r.secret == "ABC"
    finally:
        svr.shutdown()

def test_mfa_status():
    url, svr = run_mock_server({
        "GET:/api/v1/mfa/status": lambda _: (200, {"enabled": True}),
    })
    try:
        r = SSOClient(url, access_token="t").mfa_status()
        assert r.enabled is True
    finally:
        svr.shutdown()

def test_admin_health():
    url, svr = run_mock_server({
        "GET:/api/v1/admin/health": lambda _: (200, {"status": "ok", "database": "pg", "version": "1"}),
    })
    try:
        r = SSOClient(url, access_token="a").admin_health()
        assert r.status == "ok"
    finally:
        svr.shutdown()

def test_list_users():
    url, svr = run_mock_server({
        "GET:/api/v1/admin/users": lambda _: (200, {"users": [{"id": "u1"}], "total": 1, "page": 1, "page_size": 10}),
    })
    try:
        r = SSOClient(url, access_token="a").list_users(1, 10)
        assert r.total == 1
    finally:
        svr.shutdown()

def test_get_user():
    # 路径参数方式获取单个用户
    url, svr = run_mock_server({
        "GET:/api/v1/admin/users/u1": lambda _: (200, {
            "id": "u1", "email": "t@e.com", "email_verified": True,
            "mfa_enabled": False, "status": "active",
            "created_at": "2024-01-01T00:00:00Z", "updated_at": "2024-01-01T00:00:00Z",
        }),
    })
    try:
        r = SSOClient(url, access_token="a").get_user("u1")
        assert r.id == "u1"
        assert r.email == "t@e.com"
    finally:
        svr.shutdown()

def test_disable_user():
    # 路径参数方式禁用用户
    url, svr = run_mock_server({
        "POST:/api/v1/admin/users/u1/disable": lambda _: (200, {"message": "用户已禁用"}),
    })
    try:
        r = SSOClient(url, access_token="a").disable_user("u1")
        assert r.message == "用户已禁用"
    finally:
        svr.shutdown()

def test_enable_user():
    # 路径参数方式启用用户
    url, svr = run_mock_server({
        "POST:/api/v1/admin/users/u1/enable": lambda _: (200, {"message": "用户已启用"}),
    })
    try:
        r = SSOClient(url, access_token="a").enable_user("u1")
        assert r.message == "用户已启用"
    finally:
        svr.shutdown()

def test_admin_cleanup():
    url, svr = run_mock_server({
        "POST:/api/v1/admin/cleanup": lambda _: (200, {"message": "清理完成"}),
    })
    try:
        r = SSOClient(url, access_token="a").admin_cleanup()
        assert r.message == "清理完成"
    finally:
        svr.shutdown()

def test_exchange_code():
    # 服务端 authorization_code 响应包了一层 data
    url, svr = run_mock_server({
        "POST:/api/v1/token": lambda _: (200, {
            "data": {
                "access_token": "at1", "refresh_token": "rt1",
                "token_type": "Bearer", "expires_in": 3600, "scope": "read write",
            },
        }),
    })
    try:
        c = SSOClient(url)
        r = c.exchange_code("code1", "cid", "csec", "https://app.example.com/cb")
        assert r.access_token == "at1"
        assert r.refresh_token == "rt1"
        assert c.access_token == "at1"
    finally:
        svr.shutdown()

def test_exchange_code_with_pkce():
    # PKCE 场景下的授权码交换
    url, svr = run_mock_server({
        "POST:/api/v1/token": lambda _: (200, {
            "data": {
                "access_token": "at2", "refresh_token": "rt2",
                "token_type": "Bearer", "expires_in": 3600, "scope": "read",
            },
        }),
    })
    try:
        c = SSOClient(url)
        r = c.exchange_code("code2", "cid", "csec", "https://app.example.com/cb", "verifier123")
        assert r.access_token == "at2"
        assert c.access_token == "at2"
    finally:
        svr.shutdown()

def test_discovery():
    url, svr = run_mock_server({
        "GET:/.well-known/openid-configuration": lambda _: (200, {"issuer": "http://t", "grant_types_supported": ["code"]}),
    })
    try:
        r = SSOClient(url).discovery()
        assert r.issuer == "http://t"
    finally:
        svr.shutdown()

def test_jwks():
    url, svr = run_mock_server({
        "GET:/.well-known/jwks.json": lambda _: (200, {"keys": [{"kty": "RSA", "use": "sig", "kid": "k1", "n": "a", "e": "b"}]}),
    })
    try:
        r = SSOClient(url).jwks()
        assert len(r.keys) == 1
    finally:
        svr.shutdown()

def test_error_methods():
    assert SSOError(404, "N").is_not_found()
    assert SSOError(401, "U").is_unauthorized()
    assert SSOError(403, "F").is_forbidden()
    assert SSOError(409, "C").is_conflict()
    assert SSOError(429, "R").is_rate_limited()


if __name__ == "__main__":
    tests = [v for k, v in sorted(globals().items()) if k.startswith("test_")]
    passed = failed = 0
    for t in tests:
        try:
            t()
            print(f"  ✓ {t.__name__}")
            passed += 1
        except Exception as e:
            print(f"  ✗ {t.__name__}: {e}")
            failed += 1
    print(f"\n{passed} passed, {failed} failed")
    if failed:
        sys.exit(1)
