# 缓存包使用说明

## 概述

本包提供统一的缓存接口和实现，支持内存缓存和Redis缓存。

## 使用方式

### 1. 使用内存缓存（开发/测试环境）

```go
import "github.com/your-org/sso/internal/cache"

// 创建内存缓存
cache := cache.NewMemoryCache()
defer cache.Close()

// 使用缓存
ctx := context.Background()
cache.Set(ctx, "key", value, 5*time.Minute)
var result MyType
cache.Get(ctx, "key", &result)
```

### 2. 使用Redis缓存（生产环境）

```go
import "github.com/your-org/sso/internal/cache"

// 创建Redis缓存
redisCache, err := cache.NewRedisCache("localhost:6379", "", 0)
if err != nil {
    log.Fatal(err)
}
defer redisCache.Close()

// 使用方式与内存缓存相同
```

### 3. 缓存键生成

```go
// Token缓存键
key := cache.TokenKey("access-token-123")  // "token:access-token-123"

// 用户缓存键
key := cache.UserIDKey("user-123")         // "user:user-123"
key := cache.UserEmailKey("test@example.com") // "user:email:test@example.com"

// 客户端缓存键
key := cache.ClientKey("client-123")       // "client:client-123"
```

### 4. 缓存TTL配置

```go
cache.DefaultTTL  // 5分钟
cache.TokenTTL    // 15分钟
cache.ClientTTL   // 1小时
```

## 注意事项

1. Redis缓存需要安装Redis服务并配置连接信息
2. 内存缓存仅适用于开发和测试环境，不适合多实例部署
3. 缓存键命名遵循统一规范，便于管理和清理
