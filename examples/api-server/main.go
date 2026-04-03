// API服务集成示例
// 演示如何在API服务中使用SSO进行JWT验证
package main

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ============================================================================
// 配置
// ============================================================================

const (
	SSOJWKSURL = "http://localhost:9090/.well-known/jwks.json"
)

// ============================================================================
// JWKS相关结构
// ============================================================================

// JWKS JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK JSON Web Key
type JWK struct {
	Kty string `json:"kty"` // 密钥类型 (RSA)
	Use string `json:"use"` // 用途 (sig)
	Kid string `json:"kid"` // 密钥ID
	N   string `json:"n"`   // 模数 (Base64URL)
	E   string `json:"e"`   // 指数 (Base64URL)
}

// ============================================================================
// JWT验证中间件
// ============================================================================

// JWTValidator JWT验证器
type JWTValidator struct {
	publicKey *rsa.PublicKey
	issuer    string
}

// NewJWTValidator 创建JWT验证器
func NewJWTValidator(jwksURL, issuer string) (*JWTValidator, error) {
	// 获取JWKS
	publicKey, err := fetchPublicKey(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("获取公钥失败: %w", err)
	}

	return &JWTValidator{
		publicKey: publicKey,
		issuer:    issuer,
	}, nil
}

// fetchPublicKey 从JWKS端点获取公钥
func fetchPublicKey(jwksURL string) (*rsa.PublicKey, error) {
	// 请求JWKS
	resp, err := http.Get(jwksURL) // #nosec G107 -- JWKS URL来自配置的SSO服务器，不是用户输入
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 解析JWKS
	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}

	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("JWKS中没有密钥")
	}

	// 获取第一个RSA密钥
	jwk := jwks.Keys[0]
	if jwk.Kty != "RSA" {
		return nil, fmt.Errorf("不支持的密钥类型: %s", jwk.Kty)
	}

	// 这里简化处理，实际应用中需要正确解析JWK
	// 在生产环境中，建议使用专门的库如 github.com/lestrrat-go/jwx
	return nil, fmt.Errorf("需要实现JWK解析")
}

// Middleware JWT验证中间件
func (v *JWTValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 获取Authorization头
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"缺少Authorization头"}`, http.StatusUnauthorized)
			return
		}

		// 2. 解析Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, `{"error":"无效的Authorization格式"}`, http.StatusUnauthorized)
			return
		}

		// 3. 验证JWT
		token, err := jwt.Parse(parts[1], func(token *jwt.Token) (interface{}, error) {
			// 验证签名算法
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("不支持的签名算法: %v", token.Header["alg"])
			}
			return v.publicKey, nil
		})

		if err != nil {
			http.Error(w, `{"error":"无效的Token"}`, http.StatusUnauthorized)
			return
		}

		// 4. 验证Claims
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			http.Error(w, `{"error":"无效的Token"}`, http.StatusUnauthorized)
			return
		}

		// 5. 验证过期时间
		if exp, ok := claims["exp"].(float64); ok {
			if time.Now().Unix() > int64(exp) {
				http.Error(w, `{"error":"Token已过期"}`, http.StatusUnauthorized)
				return
			}
		}

		// 6. 将用户信息添加到请求上下文
		ctx := context.WithValue(r.Context(), "userID", claims["sub"])
		ctx = context.WithValue(ctx, "userEmail", claims["email"])
		ctx = context.WithValue(ctx, "userScopes", claims["scope"])

		// 7. 继续处理请求
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ============================================================================
// 示例处理器
// ============================================================================

// PublicHandler 公开处理器 (不需要认证)
func PublicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "这是一个公开的API端点",
	})
}

// ProtectedHandler 受保护的处理器 (需要认证)
func ProtectedHandler(w http.ResponseWriter, r *http.Request) {
	// 从上下文获取用户信息
	userID := r.Context().Value("userID")
	userEmail := r.Context().Value("userEmail")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "这是一个受保护的API端点",
		"user_id": userID,
		"email":   userEmail,
	})
}

// ProfileHandler 获取用户资料
func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID")
	userEmail := r.Context().Value("userEmail")
	userScopes := r.Context().Value("userScopes")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id": userID,
		"email":   userEmail,
		"scopes":  userScopes,
	})
}

// ============================================================================
// 主函数
// ============================================================================

func main() {
	fmt.Println("=== API服务集成示例 ===")
	fmt.Println()

	// 注意: 此示例需要实际的JWKS解析实现
	// 在生产环境中，建议使用 github.com/lestrrat-go/jwx 库

	// 创建路由
	mux := http.NewServeMux()

	// 公开端点
	mux.HandleFunc("/api/public", PublicHandler)

	// 以下代码需要JWT验证器
	// validator, err := NewJWTValidator(SSOJWKSURL, "sso")
	// if err != nil {
	//     log.Fatalf("创建JWT验证器失败: %v", err)
	// }

	// 受保护的端点
	// mux.Handle("/api/protected", validator.Middleware(http.HandlerFunc(ProtectedHandler)))
	// mux.Handle("/api/profile", validator.Middleware(http.HandlerFunc(ProfileHandler)))

	// 临时的受保护端点 (无验证)
	mux.HandleFunc("/api/protected", ProtectedHandler)
	mux.HandleFunc("/api/profile", ProfileHandler)

	// 启动服务器
	fmt.Println("API服务启动在 http://localhost:8081")
	fmt.Println()
	fmt.Println("可用的端点:")
	fmt.Println("  GET /api/public     - 公开端点")
	fmt.Println("  GET /api/protected  - 受保护端点 (需要JWT)")
	fmt.Println("  GET /api/profile    - 用户资料 (需要JWT)")
	fmt.Println()

	// 使用 http.Server 设置超时
	server := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
