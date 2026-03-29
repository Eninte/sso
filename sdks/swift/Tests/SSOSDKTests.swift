import XCTest
@testable import SSOSDK

final class SSOSDKTests: XCTestCase {

    func testCreateClient() async {
        let client = SSOClient(baseURL: "http://localhost:9090")
        XCTAssertEqual(await client.currentAccessToken, "")
    }

    func testSetClearTokens() async {
        let client = SSOClient(baseURL: "http://localhost:9090")
        await client.setTokens(accessToken: "a", refreshToken: "b", expiresIn: 900)
        XCTAssertEqual(await client.currentAccessToken, "a")
        await client.clearTokens()
        XCTAssertEqual(await client.currentAccessToken, "")
    }

    func testUserInfoNoToken() async {
        let client = SSOClient(baseURL: "http://localhost:9090")
        do {
            let _ = try await client.userInfo()
            XCTFail("should throw")
        } catch let e as SSOError {
            XCTAssertTrue(e.isUnauthorized())
        } catch {
            XCTFail("unexpected error: \(error)")
        }
    }

    func testErrorMethods() {
        let e404 = SSOError(httpStatus: 404, code: "NOT_FOUND", message: "", rawBody: "")
        XCTAssertTrue(e404.isNotFound())
        XCTAssertFalse(e404.isUnauthorized())

        let e401 = SSOError(httpStatus: 401, code: "", message: "", rawBody: "")
        XCTAssertTrue(e401.isUnauthorized())

        let e409 = SSOError(httpStatus: 409, code: "", message: "", rawBody: "")
        XCTAssertTrue(e409.isConflict())

        let e429 = SSOError(httpStatus: 429, code: "", message: "", rawBody: "")
        XCTAssertTrue(e429.isRateLimited())
    }

    func testErrorParse() {
        let e = SSOError.parse(httpStatus: 401, body: #"{"code":"INVALID_CREDENTIALS","message":"wrong"}"#)
        XCTAssertEqual(e.code, "INVALID_CREDENTIALS")
        XCTAssertEqual(e.message, "wrong")
    }

    func testModelDecoding() throws {
        let json = #"{"access_token":"a1","refresh_token":"r1","token_type":"Bearer","expires_in":900}"#
        let resp = try JSONDecoder().decode(TokenResponse.self, from: json.data(using: .utf8)!)
        XCTAssertEqual(resp.accessToken, "a1")
        XCTAssertEqual(resp.expiresIn, 900)
    }

    func testUserItemDecoding() throws {
        let json = #"{"id":"u1","email":"a@b.com","email_verified":true,"mfa_enabled":false,"status":"active"}"#
        let item = try JSONDecoder().decode(UserItem.self, from: json.data(using: .utf8)!)
        XCTAssertEqual(item.id, "u1")
        XCTAssertTrue(item.emailVerified)
    }
}
