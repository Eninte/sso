package com.sso.sdk

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*

class SSOClientTest {

    @Test
    fun testCreateClient() {
        val client = SSOClient("http://localhost:9090")
        assertEquals("", client.accessToken)
    }

    @Test
    fun testSetClearTokens() {
        val client = SSOClient("http://localhost:9090")
        client.setTokens("a", "b", 900)
        assertEquals("a", client.accessToken)
        client.clearTokens()
        assertEquals("", client.accessToken)
    }

    @Test
    fun testUserInfoNoToken() {
        val client = SSOClient("http://localhost:9090")
        try {
            client.userInfo()
            fail("should throw")
        } catch (e: SSOError) {
            assertTrue(e.isUnauthorized())
        }
    }

    @Test
    fun testErrorMethods() {
        assertTrue(SSOError(404, "N").isNotFound())
        assertTrue(SSOError(401, "U").isUnauthorized())
        assertTrue(SSOError(403, "F").isForbidden())
        assertTrue(SSOError(409, "C").isConflict())
        assertTrue(SSOError(429, "R").isRateLimited())
        assertFalse(SSOError(200, "").isNotFound())
    }

    @Test
    fun testErrorParse() {
        val e = SSOError.parse(401, """{"code":"INVALID_CREDENTIALS","message":"wrong"}""")
        assertEquals("INVALID_CREDENTIALS", e.code)
        assertEquals("wrong", e.message)
    }

    @Test
    fun testTokenResponseDeserialization() {
        val json = """{"access_token":"a1","refresh_token":"r1","token_type":"Bearer","expires_in":900}"""
        val resp = com.google.gson.Gson().fromJson(json, TokenResponse::class.java)
        assertEquals("a1", resp.accessToken)
        assertEquals(900, resp.expiresIn)
    }

    @Test
    fun testUserItemDeserialization() {
        val json = """{"id":"u1","email":"a@b.com","email_verified":true,"mfa_enabled":false,"status":"active"}"""
        val item = com.google.gson.Gson().fromJson(json, UserItem::class.java)
        assertEquals("u1", item.id)
        assertTrue(item.emailVerified)
    }
}
