# 生产代码测试污染修复计划

## 问题

`internal/crypto/password.go:52` 中使用 `os.Getenv("GO_TEST")` 检测测试环境，属于测试逻辑混入生产代码。

## 方案

用包级变量 `testMode` 替代环境变量，测试文件通过 `init()` 设置，实现零污染。

### 修改文件

| 文件 | 操作 |
|---|---|
| `internal/crypto/password.go` | 移除 `os.Getenv("GO_TEST")`，改用包级变量 |
| `internal/crypto/password_test.go` | 添加 `init()` 设置 `crypto.TestMode = true` |

### password.go 改动

```go
// 移除 "os" import

// testMode 测试模式标志，由测试文件 init() 设置
var testMode bool

// SetTestMode 设置测试模式（仅测试使用）
func SetTestMode(enabled bool) {
    testMode = enabled
}

func NewPasswordService(cost int) *PasswordService {
    if testMode {
        return &PasswordService{cost: int(bcrypt.MinCost)}
    }
    return &PasswordService{cost: NormalizeBcryptCost(cost)}
}
```

### password_test.go 改动

在文件顶部添加：
```go
func init() {
    crypto.SetTestMode(true)
}
```

### 验证

1. `go test -race ./...` 全部通过
2. `go build ./cmd/server/` 不含 `os.Getenv("GO_TEST")`
3. `grep -rn 'GO_TEST' --include='*.go'` 无结果
