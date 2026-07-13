// Package handler 提供 HTTP 请求处理器
// 本文件包含配置向导的辅助函数（数据库和 Redis 连接测试）
package handler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL驱动
	"github.com/redis/go-redis/v9"
)

// openDB 打开数据库连接（用于setup测试连接）
func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(5 * time.Second)
	return db, nil
}

// newRedisClient 创建Redis客户端（用于setup测试连接）
func newRedisClient(addr, password string, db int) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
}

func testDBConnection(ctx context.Context, dsn string) error {
	db, err := openDB(dsn)
	if err != nil {
		return fmt.Errorf("database connection failed, please check host, port, username and password")
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database connection test failed, please check network and credentials")
	}
	return nil
}

func testRedisConnection(ctx context.Context, addr, password string, db int) error {
	client := newRedisClient(addr, password, db)
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis connection failed, please check host and port")
	}
	return nil
}
