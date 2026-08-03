# 性能基准测试报告

生成时间: 西元2026年08月03日 (週一) 21時52分43秒 CST

## 缓存性能
```
redis: 2026/08/03 21:52:51 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:51 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:51 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:51 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:51 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
2026/08/03 21:52:51 INFO using memory cache mode
redis: 2026/08/03 21:52:51 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:56 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:56 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:56 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:56 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:56 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:56 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
2026/08/03 21:52:56 WARN redis connection failed, fallback to memory cache error="redis connection failed: dial tcp: lookup invalid-host: i/o timeout"
redis: 2026/08/03 21:52:57 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:57 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:36701: connect: connection refused
redis: 2026/08/03 21:52:57 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:57 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:57 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:57 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:57 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 5 attempts: dial tcp 127.0.0.1:1: connect: connection refused
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
2026/08/03 21:52:58 INFO redis cache enabled
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
redis: 2026/08/03 21:52:58 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp: lookup invalid-host: i/o timeout
goos: linux
goarch: amd64
pkg: github.com/example/sso/internal/cache
cpu: AMD Ryzen 5 5500                               
BenchmarkMemoryCache_Set-8                    	 1000000	      1328 ns/op	     333 B/op	       3 allocs/op
BenchmarkMemoryCache_Get-8                    	 1864478	      1038 ns/op	     181 B/op	       4 allocs/op
BenchmarkMemoryCache_SetGet-8                 	 1000000	      1487 ns/op	     501 B/op	       6 allocs/op
BenchmarkMemoryCache_Delete-8                 	 2979862	       922.2 ns/op	      23 B/op	       1 allocs/op
BenchmarkMemoryCache_DeletePattern-8          	   30914	     67962 ns/op	    7616 B/op	     305 allocs/op
BenchmarkMemoryCache_Parallel-8               	 2347926	       491.7 ns/op	     106 B/op	       3 allocs/op
BenchmarkMemoryCache_Parallel_Read-8          	 5083124	       577.4 ns/op	     182 B/op	       4 allocs/op
BenchmarkMemoryCache_Parallel_Write-8         	 2079474	       575.9 ns/op	      68 B/op	       3 allocs/op
BenchmarkMemoryCache_SetWithNilProtection-8   	 1000000	      1013 ns/op	     329 B/op	       2 allocs/op
BenchmarkMemoryCache_Set_LargeObject-8        	  425100	      5661 ns/op	    1884 B/op	       5 allocs/op
BenchmarkMemoryCache_Get_LargeObject-8        	   95127	     15272 ns/op	    1516 B/op	      10 allocs/op
BenchmarkMemoryCache_ConcurrentMixed-8        	 1845592	      1413 ns/op	     159 B/op	       2 allocs/op
PASS
ok  	github.com/example/sso/internal/cache	46.217s
```

## 服务性能
```
2026/08/03 21:53:34 WARN 账户因多次登录失败被锁定 user_id=user-123 attempts=5
2026/08/03 21:53:34 WARN 账户因多次登录失败被锁定 user_id=user-123 attempts=5
2026/08/03 21:53:34 WARN 账户因多次登录失败被锁定 user_id=user-123 attempts=6
2026/08/03 21:53:34 WARN 账户因多次登录失败被锁定 user_id=user-123 attempts=7
2026/08/03 21:53:34 WARN 账户因多次登录失败被锁定 user_id=user-123 attempts=8
2026/08/03 21:53:34 WARN 账户因多次登录失败被锁定 user_id=user-123 attempts=9
2026/08/03 21:53:34 WARN 账户因多次登录失败被锁定 user_id=user-123 attempts=10
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:35 ERROR MFA恢复码限流Redis写入失败，降级为内存限流 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp 127.0.0.1:41085: connect: connection refused
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:35 ERROR MFA恢复码限流Redis写入失败，降级为内存限流 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:35 ERROR MFA恢复码限流Redis写入失败，降级为内存限流 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:35 ERROR MFA恢复码限流Redis写入失败，降级为内存限流 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp 127.0.0.1:41085: connect: connection refused
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:35 ERROR MFA恢复码限流Redis写入失败，降级为内存限流 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:35 ERROR MFA恢复码限流Redis读取失败，降级为内存限流 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:35 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:35 ERROR MFA恢复码限流Redis清除失败 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp 127.0.0.1:41085: connect: connection refused
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:41085: i/o timeout
2026/08/03 21:53:36 ERROR MFA恢复码限流Redis读取失败，降级为内存限流 user_id=user-1 error="dial tcp 127.0.0.1:41085: i/o timeout"
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:33059: i/o timeout
2026/08/03 21:53:36 ERROR TOTP重放记录Redis写入失败，降级为内存记录 user_id=user-1 error="dial tcp 127.0.0.1:33059: i/o timeout"
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:33059: i/o timeout
2026/08/03 21:53:36 ERROR TOTP重放记录Redis写入失败，降级为内存记录 user_id=user-1 error="dial tcp 127.0.0.1:33059: i/o timeout"
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:36 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:36 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:36 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:36 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:36 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:36 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:36 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:37 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:37 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:38 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:38 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:38 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp 127.0.0.1:35103: connect: connection refused
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:38 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:35103: i/o timeout
2026/08/03 21:53:38 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:35103: i/o timeout" client_ip=10.8.0.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:38 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:38 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:38 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp 127.0.0.1:44841: connect: connection refused
redis: 2026/08/03 21:53:39 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:39 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:39 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:39 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:39 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:39 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:39 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:39 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:39 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:39 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:39 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:39 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:39 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:39 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 1 attempts: dial tcp 127.0.0.1:44841: connect: connection refused
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:40 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:40 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:41 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:41 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:41 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.1
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:44841: i/o timeout
2026/08/03 21:53:41 ERROR 登录限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:44841: i/o timeout" client_ip=10.8.1.2
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:46735: i/o timeout
2026/08/03 21:53:41 ERROR 邮件限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:46735: i/o timeout" email=fallback@example.com
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:46735: i/o timeout
2026/08/03 21:53:41 ERROR 邮件限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:46735: i/o timeout" email=fallback@example.com
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:46735: i/o timeout
2026/08/03 21:53:41 ERROR 邮件限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:46735: i/o timeout" email=fallback@example.com
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:46735: i/o timeout
2026/08/03 21:53:41 ERROR 邮件限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:46735: i/o timeout" email=fallback@example.com
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:46735: i/o timeout
2026/08/03 21:53:41 ERROR 邮件限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:46735: i/o timeout" email=fallback@example.com
redis: 2026/08/03 21:53:41 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:46735: i/o timeout
2026/08/03 21:53:41 ERROR 邮件限流Redis写入失败，降级为进程内内存限流 error="dial tcp 127.0.0.1:46735: i/o timeout" email=fallback@example.com
redis: 2026/08/03 21:53:42 pool.go:617: redis: connection pool: failed to dial after 2 attempts: dial tcp 127.0.0.1:38811: i/o timeout
2026/08/03 21:53:42 ERROR 邮件限流Redis读取失败 error="dial tcp 127.0.0.1:38811: i/o timeout" email=readonly@example.com
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-DPmrmYozZMA event_type=mfa_recovery_code_used user_id=user-1 client_id="" ip_address=127.0.0.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"127.0.0.1\\\"}\"" success=true timestamp=2026-08-03T21:53:42.017+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-lbbkdn7N4YI event_type=mfa_recovery_code_used user_id=user-1 client_id="" ip_address=127.0.0.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"127.0.0.1\\\"}\"" success=true timestamp=2026-08-03T21:53:42.018+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-XZcqaiEsNJA event_type=mfa_recovery_code_used user_id=user-1 client_id="" ip_address=127.0.0.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"127.0.0.1\\\"}\"" success=true timestamp=2026-08-03T21:53:42.018+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-1vPXGowuYPc event_type=mfa_recovery_code_used user_id=user-1 client_id="" ip_address=127.0.0.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"127.0.0.1\\\"}\"" success=true timestamp=2026-08-03T21:53:42.018+08:00
2026/08/03 21:53:42 ERROR internal service error operation=删除用户 error="assert.AnError general error for testing"
2026/08/03 21:53:42 WARN 审计日志channel已满，降级处理 component=audit log_id=test-id event_type=test.event user_id=user-1
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-P3YomOxQy2Q event_type=security.password_reset user_id=user-123 client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:42.021+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-5dAhlhakHIo event_type=security.password_reset user_id=user-concurrent client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:42.022+08:00
2026/08/03 21:53:42 WARN 禁用用户操作缺少审计服务（仅测试/开发场景应出现） user_id=disable-user-id
2026/08/03 21:53:42 ERROR internal service error operation=撤销用户Token error="assert.AnError general error for testing"
2026/08/03 21:53:42 WARN 禁用用户操作缺少审计服务（仅测试/开发场景应出现） user_id=admin-1
2026/08/03 21:53:42 ERROR internal service error operation=统计活跃管理员 error="assert.AnError general error for testing"
2026/08/03 21:53:42 WARN 禁用用户操作缺少审计服务（仅测试/开发场景应出现） user_id=normal-user
2026/08/03 21:53:42 ERROR internal service error operation=撤销用户Token error="assert.AnError general error for testing"
2026/08/03 21:53:42 WARN 删除用户操作缺少审计服务（仅测试/开发场景应出现） user_id=admin-1
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-afvisyDw0RE event_type=user.login user_id=test-user-1 client_id="" ip_address=127.0.0.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"test@example.com\\\"}\"" success=true timestamp=2026-08-03T21:53:42.023+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342--QX2uj2FWeE event_type=user.register user_id=test-user-2 client_id="" ip_address="" user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.033+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-CPtXf8sw7GA event_type=user.register user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"email\\\":\\\"test@example.com\\\"}\"" success=true timestamp=2026-08-03T21:53:42.033+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-MD3S8CrQJEA event_type=user.register user_id="" client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"email\\\":\\\"invalid@example.com\\\"}\"" success=false timestamp=2026-08-03T21:53:42.045+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-ZT9FpRUDtgk event_type=user.login user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"test@example.com\\\"}\"" success=true timestamp=2026-08-03T21:53:42.056+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-aVQji_aWzBw event_type=user.login user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"test@example.com\\\"}\"" success=false timestamp=2026-08-03T21:53:42.067+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-F5-FqDbPOts event_type=token.issued user_id=user-1 client_id=client-1 ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.078+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-ogDzdPfgzXg event_type=oauth.code_created user_id=user-1 client_id=client-1 ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.089+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-uICvJMnvVa4 event_type=user.logout user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.089+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-o_-0Ut3JgY4 event_type=token.refresh user_id=user-1 client_id=client-1 ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.100+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-BPwMRascANk event_type=security.password_changed user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.113+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-1Q1is0fvUCU event_type=security.password_reset user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.124+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-XI_hTiuaE8M event_type=security.account_locked user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.134+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-LbBVkDgdy94 event_type=mfa.enabled user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.145+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-LMCNc0MDFdE event_type=mfa.disabled user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.156+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-jrK1pmJPcO8 event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"key-123\\\"}\"" success=true timestamp=2026-08-03T21:53:42.167+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-niw2cNrp5RE event_type=key.revoked user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"key-123\\\"}\"" success=true timestamp=2026-08-03T21:53:42.179+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-SqbYxPmgWic event_type=user.logout_all user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.179+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-hKwI1E12UdQ event_type=token.revoke user_id=user-1 client_id=client-1 ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.191+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-E7d_mbrQIq4 event_type=user.login_failed user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"test@example.com\\\",\\\"reason\\\":\\\"invalid password\\\"}\"" success=false timestamp=2026-08-03T21:53:42.202+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-Zn6mYPGy984 event_type=security.account_unlocked user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.213+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-3zT6QADQlMo event_type=oauth.code_used user_id=user-1 client_id=client-1 ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.225+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-WOrJy3G9Fsw event_type=oauth.code_invalid user_id=user-1 client_id=client-1 ip_address=192.168.1.1 user_agent="" details="\"{\\\"reason\\\":\\\"invalid code\\\"}\"" success=false timestamp=2026-08-03T21:53:42.235+08:00
2026/08/03 21:53:42 INFO 审计事件 component=audit id=20260803215342-pyLu-41rxow event_type=mfa.setup user_id=user-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"\"" success=true timestamp=2026-08-03T21:53:42.246+08:00
2026/08/03 21:53:42 ERROR internal store error error="database connection failed"
2026/08/03 21:53:42 ERROR internal store error error="SQL error: connection to postgres://admin:secret@db:5432/sso failed"
2026/08/03 21:53:43 ERROR internal service error operation=检查邮箱 error="database connection failed"
2026/08/03 21:53:43 ERROR internal service error operation=创建用户 error="database write failed"
2026/08/03 21:53:43 ERROR internal service error operation=检查邮箱 error="SQL error: connection to postgres://admin:secret@db:5432/sso failed"
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-wCfP30R5p40 event_type=mfa_challenge_issued user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"expires_in\\\":300,\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.510+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-e9-Nd8Y7jSU event_type=user.login user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-login@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":true,\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.510+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-4iMIaRB1vWw event_type=user.login user_id=non-mfa-user client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"email\\\":\\\"non-mfa@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":false,\\\"user_agent\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:43.643+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-HrKo_etDCrk event_type=mfa_challenge_issued user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"expires_in\\\":300,\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.701+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-b81KqnZ81cY event_type=user.login user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-verify@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":true,\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.701+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-sSrtdhqBz_c event_type=mfa_login_totp_success user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\"}\"" success=true timestamp=2026-08-03T21:53:43.701+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-lr5RezHCz7o event_type=mfa_login_success user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-verify@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"method\\\":\\\"totp\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.702+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-BkuhYImU80U event_type=mfa_challenge_issued user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"expires_in\\\":300,\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.775+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-i311ct4dnts event_type=user.login user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-invalid@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":true,\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.775+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-gWVUAgDtB6A event_type=mfa_challenge_issued user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"expires_in\\\":300,\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.829+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-Z8EHyWhiQvw event_type=user.login user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-ip@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":true,\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.829+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-yvpx13yw4gc event_type=mfa_challenge_context_mismatch user_id=mfa-auth-user-id client_id="" ip_address="" user_agent=Mozilla/5.0 details="\"{\\\"actual_ip\\\":\\\"10.0.0.99\\\",\\\"expected_ip\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.829+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-FJinvqZx_Q8 event_type=mfa_challenge_issued user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"expires_in\\\":300,\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.946+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-hJmIMfLH2AE event_type=user.login user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-ua@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":true,\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.946+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-JPK-tAjBmcU event_type=mfa_challenge_context_mismatch user_id=mfa-auth-user-id client_id="" ip_address="" user_agent=Chrome/100.0 details="\"{\\\"actual_ip\\\":\\\"192.168.1.1\\\",\\\"expected_ip\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Chrome/100.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.946+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-BVDDsveyGBA event_type=mfa_challenge_issued user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"expires_in\\\":300,\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.991+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-CawM0PXFQOA event_type=user.login user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-onetime@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":true,\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.991+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-XS_vbxZWeNU event_type=mfa_login_totp_success user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\"}\"" success=true timestamp=2026-08-03T21:53:43.991+08:00
2026/08/03 21:53:43 INFO 审计事件 component=audit id=20260803215343-buP1-UwOxw0 event_type=mfa_login_success user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"mfa-onetime@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"method\\\":\\\"totp\\\",\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:43.992+08:00
2026/08/03 21:53:44 ERROR 用户启用 MFA 但 AuthService 未装配 MFA 服务 user_id=mfa-auth-user-id email=no-mfa-service@example.com
2026/08/03 21:53:44 INFO 审计事件 component=audit id=20260803215344-lFk0haEp790 event_type=user.login user_id=mfa-auth-user-id client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"email\\\":\\\"no-mfa-service@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":true,\\\"user_agent\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:44.170+08:00
2026/08/03 21:53:44 WARN 账户因多次登录失败被锁定 user_id=fail-user-id attempts=5
2026/08/03 21:53:44 ERROR RefreshToken: 查询Token失败 error="NOT_FOUND: 资源不存在" refresh_token_length=21
2026/08/03 21:53:44 INFO 审计事件 component=audit id=20260803215344-RzB9Cny-dmo event_type=user.login user_id=test-user-audit-login client_id="" ip_address=192.168.1.1 user_agent=Mozilla/5.0 details="\"{\\\"email\\\":\\\"auditlogin@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"mfa_required\\\":false,\\\"user_agent\\\":\\\"Mozilla/5.0\\\"}\"" success=true timestamp=2026-08-03T21:53:44.566+08:00
2026/08/03 21:53:44 INFO 审计事件 component=audit id=20260803215344-1loMpmZftsk event_type=user.logout user_id=test-user-logout client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\"}\"" success=true timestamp=2026-08-03T21:53:44.665+08:00
2026/08/03 21:53:44 INFO 审计事件 component=audit id=20260803215344-OUPcMzCEsAs event_type=user.logout_all user_id=test-user-logoutall client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\"}\"" success=true timestamp=2026-08-03T21:53:44.730+08:00
2026/08/03 21:53:44 ERROR 撤销所有Token失败 error="assert.AnError general error for testing" user_id=test-user-revoke-fail
2026/08/03 21:53:44 ERROR internal service error operation=登出所有设备 error="assert.AnError general error for testing"
2026/08/03 21:53:44 INFO 审计事件 component=audit id=20260803215344-bsmSNP0hvcQ event_type=token.refresh user_id=test-user-refresh client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"192.168.1.1\\\"}\"" success=true timestamp=2026-08-03T21:53:44.848+08:00
2026/08/03 21:53:44 ERROR RefreshToken: 查询Token失败 error="NOT_FOUND: 资源不存在" refresh_token_length=21
2026/08/03 21:53:44 ERROR internal service error operation=检查邮箱 error="database connection lost"
2026/08/03 21:53:45 ERROR internal service error operation=创建用户 error="disk full"
2026/08/03 21:53:45 ERROR internal store error error="database timeout"
2026/08/03 21:53:45 ERROR RefreshToken: 查询Token失败 error="database error" refresh_token_length=18
2026/08/03 21:53:45 ERROR RefreshToken: 查询用户失败 error="user not found in db" user_id=user-1
2026/08/03 21:53:45 ERROR internal service error operation=查询用户 error="user not found in db"
2026/08/03 21:53:45 WARN operation failed, retrying attempt=1 max_retries=3 delay=110.190841ms error="token table locked"
2026/08/03 21:53:45 WARN operation failed, retrying attempt=2 max_retries=3 delay=218.21013ms error="token table locked"
2026/08/03 21:53:45 ERROR 登出时撤销Token失败 error="operation failed after 3 retries: token table locked" token_prefix=some-acc...
2026/08/03 21:53:45 ERROR internal service error operation=登出 error="operation failed after 3 retries: token table locked"
2026/08/03 21:53:45 ERROR 撤销所有Token失败 error="database error" user_id=user-123
2026/08/03 21:53:45 ERROR internal service error operation=登出所有设备 error="database error"
2026/08/03 21:53:45 INFO 审计事件 component=audit id=20260803215345-3BN2drLIpO4 event_type=user.login user_id=audit-login-user client_id="" ip_address=192.168.1.100 user_agent=TestAgent/1.0 details="\"{\\\"email\\\":\\\"auditlogin@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.100\\\",\\\"mfa_required\\\":false,\\\"user_agent\\\":\\\"TestAgent/1.0\\\"}\"" success=true timestamp=2026-08-03T21:53:45.615+08:00
2026/08/03 21:53:45 INFO 审计事件 component=audit id=20260803215345-qUJeTGaYUA4 event_type=user.login_failed user_id=audit-login-fail-user client_id="" ip_address=10.0.0.1 user_agent="" details="\"{\\\"email\\\":\\\"auditfail@example.com\\\",\\\"ip_address\\\":\\\"10.0.0.1\\\",\\\"success\\\":false,\\\"user_agent\\\":\\\"\\\"}\"" success=false timestamp=2026-08-03T21:53:45.617+08:00
2026/08/03 21:53:45 INFO 审计事件 component=audit id=20260803215345-BG9iebVnj_c event_type=user.logout user_id=audit-logout-user client_id="" ip_address=172.16.0.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"172.16.0.1\\\"}\"" success=true timestamp=2026-08-03T21:53:45.767+08:00
2026/08/03 21:53:45 INFO 审计事件 component=audit id=20260803215345-kbG0c_ddfmw event_type=token.refresh user_id=audit-refresh-user client_id="" ip_address=192.168.2.1 user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"192.168.2.1\\\"}\"" success=true timestamp=2026-08-03T21:53:45.795+08:00
2026/08/03 21:53:45 WARN 清除Token缓存失败 error=缓存服务暂时不可用 token=test-acc...
2026/08/03 21:53:45 WARN operation failed, retrying attempt=1 max_retries=3 delay=105.9931ms error="database lock timeout"
2026/08/03 21:53:46 WARN operation failed, retrying attempt=2 max_retries=3 delay=243.992292ms error="database lock timeout"
2026/08/03 21:53:46 ERROR 登出时撤销Token失败 error="operation failed after 3 retries: database lock timeout" token_prefix=test-acc...
2026/08/03 21:53:46 ERROR internal service error operation=登出 error="operation failed after 3 retries: database lock timeout"
2026/08/03 21:53:46 ERROR 登出时撤销Token失败 error="context canceled" token_prefix=test-acc...
2026/08/03 21:53:46 ERROR internal service error operation=登出 error="context canceled"
2026/08/03 21:53:46 WARN RefreshToken: Token 已撤销，触发重放防御 token_id=token-rotation-1 revoked_at=2026-08-03T21:53:46.683784642+08:00
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=15
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-B7LC5b8fpOg event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":15}\"" success=true timestamp=2026-08-03T21:53:46.683+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: Token 已撤销，触发重放防御 token_id=token-rotation-1 revoked_at=2026-08-03T21:53:46.732773011+08:00
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-NxIseUuvkPM event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.732+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: Token 已撤销，触发重放防御 token_id=token-rotation-1 revoked_at=2026-08-03T21:53:46.732773011+08:00
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-a78X3-UwG5E event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.732+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求） token_id=token-rotation-1 user_id=user-rotation
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-BIqMdrVbbQQ event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.733+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求） token_id=token-rotation-1 user_id=user-rotation
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-i7osf_Ascek event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.733+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求） token_id=token-rotation-1 user_id=user-rotation
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-hKWVjmCX3nc event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.733+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求） token_id=token-rotation-1 user_id=user-rotation
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 WARN RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求） token_id=token-rotation-1 user_id=user-rotation
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 WARN RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求） token_id=token-rotation-1 user_id=user-rotation
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-3gxoXWI7Wgk event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.733+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-f63hymJb0_U event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.733+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: 检测到 Token 已被轮换（重放攻击或并发请求） token_id=token-rotation-1 user_id=user-rotation
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=18
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-4Jldx3bUjgc event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.733+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-E0YtOMIz0LE event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":18}\"" success=true timestamp=2026-08-03T21:53:46.733+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: Token 已撤销，触发重放防御 token_id=token-rotation-1 revoked_at=2026-08-03T21:53:46.807797449+08:00
2026/08/03 21:53:46 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=15
2026/08/03 21:53:46 INFO 关键审计事件（同步） component=audit id=20260803215346-t9-24JXZaYw event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":15}\"" success=true timestamp=2026-08-03T21:53:46.807+08:00
2026/08/03 21:53:46 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:46 WARN RefreshToken: Refresh Token 已过期 token_id=token-expired-1 refresh_expires_at=2026-08-03T20:53:46.84499416+08:00
2026/08/03 21:53:46 WARN RefreshToken: Refresh Token 已过期 token_id=token-legacy-2 refresh_expires_at=2026-08-03T20:53:46.944854465+08:00
2026/08/03 21:53:46 WARN RefreshToken: 用户已被禁用 user_id=user-rotation
2026/08/03 21:53:46 WARN RefreshToken: 用户已被锁定 user_id=user-rotation
2026/08/03 21:53:47 WARN RefreshToken: Token 已撤销，触发重放防御 token_id=token-rotation-1 revoked_at=2026-08-03T21:53:47.067453673+08:00
2026/08/03 21:53:47 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-rotation refresh_token_length=25
2026/08/03 21:53:47 INFO 关键审计事件（同步） component=audit id=20260803215347-vI_lUMZIXWs event_type=security.suspicious user_id=user-rotation client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":25}\"" success=true timestamp=2026-08-03T21:53:47.067+08:00
2026/08/03 21:53:47 INFO 关键审计日志已记录 event=security.suspicious user_id=user-rotation success=true
2026/08/03 21:53:47 ERROR 重放攻击下撤销全部 Token 失败 error="database unavailable" user_id=user-rotation
2026/08/03 21:53:47 ERROR internal service error operation=撤销全部Token error="database unavailable"
2026/08/03 21:53:47 INFO 审计事件 component=audit id=20260803215347-myyWuEsJTBI event_type=system.start user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"version\\\":\\\"1.0.0\\\"}\"" success=true timestamp=2026-08-03T21:53:47.067+08:00
2026/08/03 21:53:47 INFO 邮件发送成功 component=email to=to***@example.com subject="Test Subject"
2026/08/03 21:53:47 INFO 邮件发送成功 component=email to=to***@example.com subject="SSL Test"
2026/08/03 21:53:47 ERROR 发送邮件失败 component=email to=to***@example.com error="assert.AnError general error for testing"
2026/08/03 21:53:47 INFO 邮件发送成功 component=email to=u***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:53:47 INFO 邮件发送成功 component=email to=u***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:53:47 ERROR 发送邮件失败 component=email to=u***@example.com error="assert.AnError general error for testing"
2026/08/03 21:53:47 INFO 邮件发送成功 component=email to=u***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:53:47 INFO 邮件发送成功 component=email to=u***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:53:47 ERROR 发送邮件失败 component=email to=u***@example.com error="assert.AnError general error for testing"
2026/08/03 21:53:47 ERROR 发送邮件失败 component=email to=to***@example.com error="dial tcp [::1]:1: connect: connection refused"
2026/08/03 21:53:47 ERROR 发送邮件失败 component=email to=to***@example.com error="dial tcp [::1]:2: connect: connection refused"
2026/08/03 21:53:47 ERROR 发送邮件失败 component=email to=u***@example.com error="dial tcp [::1]:1: connect: connection refused"
2026/08/03 21:53:47 ERROR 发送邮件失败 component=email to=u***@example.com error="dial tcp [::1]:1: connect: connection refused"
2026/08/03 21:53:47 ERROR internal service error operation=查询管理员状态 error="database error"
2026/08/03 21:53:47 INFO 关键审计事件（同步） component=audit id=20260803215347-KbZV9h7SK1w event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"4u8b5GxWQL8s5a5S\\\"}\"" success=true timestamp=2026-08-03T21:53:47.345+08:00
2026/08/03 21:53:47 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:47 INFO key rotation completed new_key_id=4u8b5GxWQL8s5a5S transition_period=24h0m0s
2026/08/03 21:53:47 INFO 关键审计事件（同步） component=audit id=20260803215347-mqVCPsoKP0M event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"tr3BA-qPHw70etDO\\\"}\"" success=true timestamp=2026-08-03T21:53:47.445+08:00
2026/08/03 21:53:47 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:47 INFO key rotation completed new_key_id=tr3BA-qPHw70etDO transition_period=24h0m0s
2026/08/03 21:53:49 INFO old key deprecated key_id=tr3BA-qPHw70etDO expires_at=2026-08-04T21:53:49.102+08:00
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-jJtwrzAYDQ4 event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"z9Od2zvYmX9pwpjS\\\"}\"" success=true timestamp=2026-08-03T21:53:49.102+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=z9Od2zvYmX9pwpjS transition_period=24h0m0s
2026/08/03 21:53:49 INFO revoked expired key key_id=expired-key-1
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-pKbDiS3r9is event_type=key.revoked user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"expired-key-1\\\"}\"" success=true timestamp=2026-08-03T21:53:49.117+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.revoked user_id="" success=true
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-OPxPVe8CybE event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"7x1OL0MCwAihwXlA\\\"}\"" success=true timestamp=2026-08-03T21:53:49.218+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=7x1OL0MCwAihwXlA transition_period=24h0m0s
2026/08/03 21:53:49 INFO old key deprecated key_id=7x1OL0MCwAihwXlA expires_at=2026-08-04T21:53:49.332+08:00
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-d2rHg2pAYO4 event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"PlDrxgdLKgCTQUvC\\\"}\"" success=true timestamp=2026-08-03T21:53:49.332+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=PlDrxgdLKgCTQUvC transition_period=24h0m0s
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-kW6CKplI60A event_type=key.revoked user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"7x1OL0MCwAihwXlA\\\"}\"" success=true timestamp=2026-08-03T21:53:49.332+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.revoked user_id="" success=true
2026/08/03 21:53:49 INFO key revoked key_id=7x1OL0MCwAihwXlA
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-zfoEnAA09ds event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"-hlQNmIriBu-O2Q5\\\"}\"" success=true timestamp=2026-08-03T21:53:49.400+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=-hlQNmIriBu-O2Q5 transition_period=24h0m0s
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-ebtRAyFOESA event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"PNZZ92AJoK3Zzb6D\\\"}\"" success=true timestamp=2026-08-03T21:53:49.646+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=PNZZ92AJoK3Zzb6D transition_period=24h0m0s
2026/08/03 21:53:49 INFO old key deprecated key_id=PNZZ92AJoK3Zzb6D expires_at=2026-08-04T21:53:49.714+08:00
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-ZPvHYcX_DQo event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"WeMNHhQBqG_bAj6C\\\"}\"" success=true timestamp=2026-08-03T21:53:49.714+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=WeMNHhQBqG_bAj6C transition_period=24h0m0s
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-bx5uzcAKl1M event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"0pIOSZgZdXAkgpTx\\\"}\"" success=true timestamp=2026-08-03T21:53:49.906+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=0pIOSZgZdXAkgpTx transition_period=24h0m0s
2026/08/03 21:53:49 INFO 关键审计事件（同步） component=audit id=20260803215349-dMBKWmpfHxc event_type=key.rotated user_id="" client_id="" ip_address="" user_agent="" details="\"{\\\"key_id\\\":\\\"FGCuZSSfAmfwnu3v\\\"}\"" success=true timestamp=2026-08-03T21:53:49.945+08:00
2026/08/03 21:53:49 INFO 关键审计日志已记录 event=key.rotated user_id="" success=true
2026/08/03 21:53:49 INFO key rotation completed new_key_id=FGCuZSSfAmfwnu3v transition_period=24h0m0s
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-Jh0w38IngAM event_type=mfa_login_totp_success user_id=mfa-login-user-id client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\"}\"" success=true timestamp=2026-08-03T21:53:49.945+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-hjZm8HUzu2s event_type=mfa_login_inconsistent_state user_id=mfa-login-user-id client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"reason\\\":\\\"mfa_enabled_but_secret_empty\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-C4zhjCpZDPo event_type=mfa_recovery_code_used user_id=mfa-login-user-id client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-Xdzn-BeRcnM event_type=mfa.setup user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-6Vqpj9oSgGg event_type=mfa.setup user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-O60gG1fYe94 event_type=mfa.enabled user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-7t8_ktgtCrs event_type=mfa.setup user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-rugZctoUzdU event_type=mfa.enabled user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-EawfEU7w9Ls event_type=mfa.disabled user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-uoKc2SjFwok event_type=mfa.setup user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-mOyZDGtdNoc event_type=mfa.enabled user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-SXky5UCwmns event_type=mfa.setup user_id=user-1 client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-Q_D9AJGEuFA event_type=mfa.enabled user_id=user-1 client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-YUQYUybo8k4 event_type=mfa.enabled user_id=user-2 client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-zpUSnq9ATd4 event_type=mfa.setup user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-E-98B6azFBQ event_type=mfa.enabled user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:49 INFO 审计事件 component=audit id=20260803215349-SW_8EW12z3s event_type=mfa.enabled user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:53:49.946+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-T4s0g0L4sA0 event_type=oauth.code_created user_id=consent-flow-user client_id=consent-flow-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"consent-flow-client\\\",\\\"consent\\\":true}\"" success=true timestamp=2026-08-03T21:53:50.178+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-NhKRaLvIDpI event_type=oauth.code_invalid user_id=consent-flow-user client_id="" ip_address="" user_agent="" details="\"{\\\"reason\\\":\\\"consent_token_invalid\\\"}\"" success=true timestamp=2026-08-03T21:53:50.178+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-QpVbRZsvp3k event_type=oauth.code_invalid user_id=different-user-id client_id=consent-flow-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"consent-flow-client\\\",\\\"reason\\\":\\\"consent_user_mismatch\\\"}\"" success=true timestamp=2026-08-03T21:53:50.179+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-w7pHNjUb8kc event_type=oauth.code_invalid user_id=consent-flow-user client_id=public-consent-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"public-consent-client\\\",\\\"reason\\\":\\\"pkce_invalid_in_consent\\\"}\"" success=true timestamp=2026-08-03T21:53:50.182+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-xLw0Ogh7-6o event_type=oauth.code_invalid user_id=consent-flow-user client_id=consent-flow-client ip_address="" user_agent="" details="\"{\\\"actual_state_length\\\":21,\\\"client_id\\\":\\\"consent-flow-client\\\",\\\"expected_state_length\\\":18,\\\"reason\\\":\\\"consent_state_mismatch\\\"}\"" success=true timestamp=2026-08-03T21:53:50.183+08:00
2026/08/03 21:53:50 WARN RefreshToken: client_id与Token归属不一致 token_id=token-2 token_client_id=oauth-client-1 request_client_id=different-client-id
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-Ill70c3HQ-Q event_type=security.suspicious user_id=user-2 client_id="" ip_address="" user_agent="" details="\"{\\\"reason\\\":\\\"refresh_token_client_mismatch\\\",\\\"request_client_id\\\":\\\"different-client-id\\\",\\\"token_client_id\\\":\\\"oauth-client-1\\\"}\"" success=true timestamp=2026-08-03T21:53:50.245+08:00
2026/08/03 21:53:50 WARN RefreshToken: OAuth签发的Token未传client_id token_id=token-3 token_client_id=oauth-client-id
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-SpDq2GlSL6k event_type=oauth.code_invalid user_id=user-1 client_id=sec-public-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"sec-public-client\\\",\\\"reason\\\":\\\"pkce_required\\\"}\"" success=true timestamp=2026-08-03T21:53:50.317+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-exngeevvBng event_type=oauth.code_invalid user_id=user-1 client_id=sec-public-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"sec-public-client\\\",\\\"reason\\\":\\\"pkce_required\\\"}\"" success=true timestamp=2026-08-03T21:53:50.317+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-AQ-XRMEl1ss event_type=oauth.code_invalid user_id=user-1 client_id=sec-public-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"sec-public-client\\\",\\\"reason\\\":\\\"invalid_scope\\\"}\"" success=true timestamp=2026-08-03T21:53:50.317+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-SBmUQcqUJfw event_type=oauth.code_created user_id=user-1 client_id=sec-public-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"sec-public-client\\\"}\"" success=true timestamp=2026-08-03T21:53:50.317+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-8XqNDkZf2Dc event_type=oauth.code_invalid user_id=user-1 client_id=sec-conf-client ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"sec-conf-client\\\",\\\"reason\\\":\\\"pkce_required\\\"}\"" success=true timestamp=2026-08-03T21:53:50.318+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-oN9ZnuW6BnI event_type=oauth.code_created user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.423+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-SPIMScR6bSA event_type=oauth.code_created user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.423+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-6fYcXXry3EY event_type=oauth.code_invalid user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\",\\\"reason\\\":\\\"pkce_required\\\"}\"" success=true timestamp=2026-08-03T21:53:50.423+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-DAGVS6__1N8 event_type=oauth.code_created user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.569+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-8YzdGdgd_Q8 event_type=oauth.code_used user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.629+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-Ngm1KVe5HLs event_type=oauth.code_created user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.630+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-264nAR7imyg event_type=oauth.code_used user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.688+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-Nl9sCddS1tE event_type=oauth.code_invalid user_id="" client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"invalid_code\\\"}\"" success=true timestamp=2026-08-03T21:53:50.689+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-XhnI3Zfi7xE event_type=oauth.code_created user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.689+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-sRIgCHCWnHU event_type=oauth.code_invalid user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"invalid_client_secret\\\"}\"" success=true timestamp=2026-08-03T21:53:50.748+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-FIVy7rncEbo event_type=oauth.code_created user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.749+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-eiEmmhbZrbI event_type=oauth.code_used user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.807+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-xF_UdaeSIO4 event_type=oauth.code_invalid user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"code_used\\\"}\"" success=true timestamp=2026-08-03T21:53:50.809+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-nGpS80fqlgM event_type=oauth.code_created user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\"}\"" success=true timestamp=2026-08-03T21:53:50.809+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-OlKGwLtfSDc event_type=oauth.code_invalid user_id=test-user-id client_id=test-client-id ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"test-client-id\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"pkce_verification_failed\\\"}\"" success=true timestamp=2026-08-03T21:53:50.868+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-SIOUKZeA6YE event_type=token.revoke user_id="" client_id="" ip_address="" user_agent="" details="\"{}\"" success=true timestamp=2026-08-03T21:53:50.910+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-ErfhMZ3YlE0 event_type=token.revoke user_id="" client_id="" ip_address="" user_agent="" details="\"{}\"" success=true timestamp=2026-08-03T21:53:50.910+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-CFwA02hxG7M event_type=token.revoke user_id="" client_id="" ip_address="" user_agent="" details="\"{}\"" success=true timestamp=2026-08-03T21:53:50.910+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-UQ6MBZLtnQA event_type=token.revoke user_id="" client_id="" ip_address="" user_agent="" details="\"{}\"" success=true timestamp=2026-08-03T21:53:50.910+08:00
2026/08/03 21:53:50 INFO 审计事件 component=audit id=20260803215350-M88yOJMYvHA event_type=token.revoke user_id="" client_id="" ip_address="" user_agent="" details="\"{}\"" success=true timestamp=2026-08-03T21:53:50.912+08:00
2026/08/03 21:53:50 ERROR internal service error operation=撤销Token error="assert.AnError general error for testing"
2026/08/03 21:53:51 ERROR internal service error operation=查询用户 error="database connection failed"
2026/08/03 21:53:51 ERROR internal service error operation=更新用户 error="database write failed"
2026/08/03 21:53:51 ERROR internal service error operation=查询用户 error="SQL error: connection to postgres://admin:secret@db:5432/sso failed"
2026/08/03 21:53:51 ERROR internal service error operation=查询用户 error="database connection failed"
2026/08/03 21:53:51 ERROR internal service error operation=查询用户 error="database connection failed"
2026/08/03 21:53:51 ERROR internal service error operation=查询用户MFA状态 error="database connection failed"
2026/08/03 21:53:51 WARN RefreshToken: Token 已撤销，触发重放防御 token_id=token-1 revoked_at=2026-08-03T21:53:51.077174838+08:00
2026/08/03 21:53:51 ERROR 检测到 Refresh Token 重放攻击，撤销用户全部 Token user_id=user-1 refresh_token_length=21
2026/08/03 21:53:51 INFO 关键审计事件（同步） component=audit id=20260803215351-b-NPous7sys event_type=security.suspicious user_id=user-1 client_id="" ip_address="" user_agent="" details="\"{\\\"client_id\\\":\\\"\\\",\\\"ip_address\\\":\\\"\\\",\\\"reason\\\":\\\"refresh_token_replay\\\",\\\"refresh_token_len\\\":21}\"" success=true timestamp=2026-08-03T21:53:51.094+08:00
2026/08/03 21:53:51 INFO 关键审计日志已记录 event=security.suspicious user_id=user-1 success=true
2026/08/03 21:53:51 ERROR RefreshToken: 原子轮换失败 error="database connection lost" token_id=token-1 user_id=user-1
2026/08/03 21:53:51 ERROR internal service error operation=轮换RefreshToken error="database connection lost"
2026/08/03 21:53:51 ERROR RefreshToken: 查询Token失败 error="SQL error: connection to postgres://admin:secret@db:5432/sso failed" refresh_token_length=18
2026/08/03 21:53:51 WARN operation failed, retrying attempt=1 max_retries=3 delay=106.872549ms error="database lock timeout"
2026/08/03 21:53:51 WARN operation failed, retrying attempt=2 max_retries=3 delay=238.216769ms error="database lock timeout"
2026/08/03 21:53:51 ERROR 登出时撤销Token失败 error="operation failed after 3 retries: database lock timeout" token_prefix=some-acc...
2026/08/03 21:53:51 ERROR internal service error operation=登出 error="operation failed after 3 retries: database lock timeout"
2026/08/03 21:53:51 WARN operation failed, retrying attempt=1 max_retries=3 delay=110.143579ms error="SQL error: DELETE FROM tokens WHERE access_token='abc123' failed: connection to postgres://admin:secret@db:5432/sso failed"
2026/08/03 21:53:51 WARN operation failed, retrying attempt=2 max_retries=3 delay=220.609642ms error="SQL error: DELETE FROM tokens WHERE access_token='abc123' failed: connection to postgres://admin:secret@db:5432/sso failed"
2026/08/03 21:53:51 ERROR 登出时撤销Token失败 error="operation failed after 3 retries: SQL error: DELETE FROM tokens WHERE access_token='abc123' failed: connection to postgres://admin:secret@db:5432/sso failed" token_prefix=some-acc...
2026/08/03 21:53:51 ERROR internal service error operation=登出 error="operation failed after 3 retries: SQL error: DELETE FROM tokens WHERE access_token='abc123' failed: connection to postgres://admin:secret@db:5432/sso failed"
2026/08/03 21:53:51 ERROR 撤销所有Token失败 error="database connection failed" user_id=user-123
2026/08/03 21:53:51 ERROR internal service error operation=登出所有设备 error="database connection failed"
2026/08/03 21:53:52 ERROR 撤销所有Token失败 error="SQL error: UPDATE tokens SET revoked_at=NOW() WHERE user_id='user-123' failed: connection to postgres://admin:secret@db:5432/sso failed" user_id=user-123
2026/08/03 21:53:52 ERROR internal service error operation=登出所有设备 error="SQL error: UPDATE tokens SET revoked_at=NOW() WHERE user_id='user-123' failed: connection to postgres://admin:secret@db:5432/sso failed"
2026/08/03 21:53:52 ERROR internal service error operation=查询用户 error="database connection failed"
2026/08/03 21:53:52 ERROR internal service error operation=存储验证令牌 error="database write failed"
2026/08/03 21:53:52 ERROR internal store error error="database connection failed"
2026/08/03 21:53:52 ERROR internal service error operation=查询用户 error="database connection failed"
2026/08/03 21:53:52 ERROR internal service error operation=更新用户 error="database write failed"
2026/08/03 21:53:52 ERROR internal store error error="database connection failed"
2026/08/03 21:53:52 ERROR internal service error operation=查询用户 error="database connection failed"
2026/08/03 21:53:52 ERROR internal service error operation=更新用户 error="database write failed"
2026/08/03 21:53:52 ERROR internal service error operation=查询用户 error="database connection failed"
2026/08/03 21:53:52 INFO 审计事件 component=audit id=20260803215352-IbTI9twVg30 event_type=security.password_changed user_id=test-user-id client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\",\\\"success\\\":false}\"" success=false timestamp=2026-08-03T21:53:52.298+08:00
2026/08/03 21:53:52 ERROR internal service error operation=更新用户 error="database write failed"
2026/08/03 21:53:52 ERROR ForgotPassword: 存储重置令牌失败 error="database write failed" user_id=test-user-id
2026/08/03 21:53:52 INFO 审计事件 component=audit id=20260803215352-OS48HXljBY0 event_type=security.password_changed user_id=user-changepw-1 client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"success\\\":true}\"" success=true timestamp=2026-08-03T21:53:52.351+08:00
2026/08/03 21:53:52 INFO 审计事件 component=audit id=20260803215352-M_xmD8Vvrfg event_type=security.password_changed user_id=user-changepw-fail client_id="" ip_address=192.168.1.1 user_agent="" details="\"{\\\"ip_address\\\":\\\"192.168.1.1\\\",\\\"success\\\":false}\"" success=false timestamp=2026-08-03T21:53:52.431+08:00
2026/08/03 21:53:52 INFO 审计事件 component=audit id=20260803215352-4cT16Pjjq_4 event_type=user.login_failed user_id=user-lock-1 client_id="" ip_address=192.168.1.100 user_agent="" details="\"{\\\"email\\\":\\\"lock@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.100\\\",\\\"success\\\":false,\\\"user_agent\\\":\\\"\\\"}\"" success=false timestamp=2026-08-03T21:53:52.482+08:00
2026/08/03 21:53:52 INFO 审计事件 component=audit id=20260803215352-LKqDqUsc4zI event_type=security.account_locked user_id=user-lock-1 client_id="" ip_address=192.168.1.100 user_agent="" details="\"{\\\"attempts\\\":2,\\\"ip_address\\\":\\\"192.168.1.100\\\"}\"" success=true timestamp=2026-08-03T21:53:52.483+08:00
2026/08/03 21:53:52 WARN 账户因多次登录失败被锁定 user_id=user-lock-1 attempts=2
2026/08/03 21:53:52 INFO 审计事件 component=audit id=20260803215352-SmvC8XFGyq0 event_type=user.login_failed user_id=user-lock-1 client_id="" ip_address=192.168.1.100 user_agent="" details="\"{\\\"email\\\":\\\"lock@example.com\\\",\\\"ip_address\\\":\\\"192.168.1.100\\\",\\\"success\\\":false,\\\"user_agent\\\":\\\"\\\"}\"" success=false timestamp=2026-08-03T21:53:52.483+08:00
2026/08/03 21:53:52 ERROR internal store error error="assert.AnError general error for testing"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=t***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:54:06 INFO 审计事件 component=audit id=20260803215406-chp0bAdGfdQ event_type=security.password_reset user_id=user-reset client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\"}\"" success=true timestamp=2026-08-03T21:54:06.013+08:00
2026/08/03 21:54:06 INFO 审计事件 component=audit id=20260803215406-snvJhJndp1A event_type=security.password_changed user_id=user-123 client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\",\\\"success\\\":false}\"" success=false timestamp=2026-08-03T21:54:06.015+08:00
2026/08/03 21:54:06 INFO 审计事件 component=audit id=20260803215406-6Hueey9MBhM event_type=security.password_changed user_id=user-123 client_id="" ip_address="" user_agent="" details="\"{\\\"ip_address\\\":\\\"\\\",\\\"success\\\":true}\"" success=true timestamp=2026-08-03T21:54:06.018+08:00
2026/08/03 21:54:06 ERROR internal store error error="database error"
2026/08/03 21:54:06 ERROR internal store error error="database connection failed"
2026/08/03 21:54:06 ERROR internal service error operation=查询用户 error="database read error"
2026/08/03 21:54:06 ERROR internal service error operation=更新用户 error="database write error"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=v***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:54:06 ERROR 发送邮件失败 component=email to=f***@example.com error="assert.AnError general error for testing"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=f***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:54:06 ERROR 发送邮件失败 component=email to=f***@example.com error="assert.AnError general error for testing"
2026/08/03 21:54:06 ERROR ForgotPassword: 异步发送重置邮件失败 error="EMAIL_SEND_FAILED: 邮件发送失败，请稍后重试" user_id=user-fp-fail
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=r***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=r***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=r***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=r***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=r***@example.com subject="验证您的邮箱 - SSO服务"
2026/08/03 21:54:06 WARN 密码重置邮件发送受限 email=f***@example.com ttl_minutes=59
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=f***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=f***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=f***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=f***@example.com subject="重置您的密码 - SSO服务"
2026/08/03 21:54:06 INFO 邮件发送成功 component=email to=f***@example.com subject="重置您的密码 - SSO服务"
goos: linux
goarch: amd64
pkg: github.com/example/sso/internal/service
cpu: AMD Ryzen 5 5500                               
BenchmarkAuthService_Login-8            	     602	   4307694 ns/op	   13119 B/op	      94 allocs/op
BenchmarkAuthService_Login_Parallel-8   	    3376	   1039958 ns/op	   13129 B/op	      94 allocs/op
BenchmarkAuthService_ValidateToken-8    	   26068	     42157 ns/op	    6272 B/op	      76 allocs/op
BenchmarkAuthService_RefreshToken-8     	     398	   3009298 ns/op	   20842 B/op	     163 allocs/op
BenchmarkAuthService_Register-8         	    1272	   1060256 ns/op	    5752 B/op	      22 allocs/op
BenchmarkAuthService_LoginFlow-8        	     588	   2148526 ns/op	   25800 B/op	     248 allocs/op
PASS
ok  	github.com/example/sso/internal/service	46.502s
2026/08/03 21:54:20 WARN template not found, using default language component=email_engine requested=verification/verification_fr.html default=zh
2026/08/03 21:54:20 WARN template not found, using default language component=email_engine requested=password_reset/password_reset_de.html default=zh
PASS
ok  	github.com/example/sso/internal/service/email	0.011s
```
