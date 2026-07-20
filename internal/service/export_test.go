// Package service 内部测试导出（仅在测试时编译）
// 用于将 private 方法暴露给 package service_test 进行黑盒测试
package service

import (
	"context"

	"github.com/example/sso/internal/model"
)

// FindOrCreateSocialUserForTest 为测试导出 findOrCreateSocialUser
// 仅在测试时编译，不会进入生产二进制
func (s *SocialLoginService) FindOrCreateSocialUserForTest(
	ctx context.Context, provider string, identity *ProviderIdentity,
) (*model.User, error) {
	return s.findOrCreateSocialUser(ctx, provider, identity)
}
