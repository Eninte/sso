package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"testing"
)

// BenchmarkHMAC_SHA256 测试HMAC-SHA256性能
func BenchmarkHMAC_SHA256(b *testing.B) {
	key := []byte("test-hmac-key-32-bytes-long-xxxx")
	code := []byte("A3F2-B8D1-C4E9-7F6A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac := hmac.New(sha256.New, key)
		mac.Write(code)
		_ = mac.Sum(nil)
	}
}

// BenchmarkHMAC_SHA512 测试HMAC-SHA512性能
func BenchmarkHMAC_SHA512(b *testing.B) {
	key := []byte("test-hmac-key-32-bytes-long-xxxx")
	code := []byte("A3F2-B8D1-C4E9-7F6A")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac := hmac.New(sha512.New, key)
		mac.Write(code)
		_ = mac.Sum(nil)
	}
}

// BenchmarkHMAC_SHA256_Parallel 并发测试SHA256
func BenchmarkHMAC_SHA256_Parallel(b *testing.B) {
	key := []byte("test-hmac-key-32-bytes-long-xxxx")
	code := []byte("A3F2-B8D1-C4E9-7F6A")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mac := hmac.New(sha256.New, key)
			mac.Write(code)
			_ = mac.Sum(nil)
		}
	})
}

// BenchmarkHMAC_SHA512_Parallel 并发测试SHA512
func BenchmarkHMAC_SHA512_Parallel(b *testing.B) {
	key := []byte("test-hmac-key-32-bytes-long-xxxx")
	code := []byte("A3F2-B8D1-C4E9-7F6A")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mac := hmac.New(sha512.New, key)
			mac.Write(code)
			_ = mac.Sum(nil)
		}
	})
}
