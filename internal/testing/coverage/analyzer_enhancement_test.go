package coverage

import (
	"testing"
)

// ============================================================================
// 增强功能测试
// ============================================================================

func TestCoverageAnalyzer_GetPackagesBelowThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		report    *CoverageReport
		wantCount int
		wantFirst string // 最低覆盖率的包名
	}{
		{
			name:      "All packages above threshold",
			threshold: 70.0,
			report: &CoverageReport{
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 80,
					},
					"internal/service/auth.go": {
						FilePath:          "internal/service/auth.go",
						TotalStatements:   100,
						CoveredStatements: 75,
					},
				},
			},
			wantCount: 0,
		},
		{
			name:      "Some packages below threshold",
			threshold: 80.0,
			report: &CoverageReport{
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 70,
					},
					"internal/service/auth.go": {
						FilePath:          "internal/service/auth.go",
						TotalStatements:   100,
						CoveredStatements: 85,
					},
					"internal/store/user.go": {
						FilePath:          "internal/store/user.go",
						TotalStatements:   100,
						CoveredStatements: 60,
					},
				},
			},
			wantCount: 2,
			wantFirst: "internal/store", // 最低覆盖率 (60%)
		},
		{
			name:      "Multiple files in same package",
			threshold: 80.0,
			report: &CoverageReport{
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 70,
					},
					"internal/handler/user.go": {
						FilePath:          "internal/handler/user.go",
						TotalStatements:   100,
						CoveredStatements: 60,
					},
				},
			},
			wantCount: 1,                  // 同一个包
			wantFirst: "internal/handler", // 平均覆盖率 65%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewCoverageAnalyzer(tt.threshold, nil)
			deficits := analyzer.GetPackagesBelowThreshold(tt.report)

			if len(deficits) != tt.wantCount {
				t.Errorf("GetPackagesBelowThreshold() count = %d, want %d", len(deficits), tt.wantCount)
			}

			if tt.wantCount > 0 && len(deficits) > 0 {
				// 检查排序（最低覆盖率应该在前面）
				if deficits[0].PackagePath != tt.wantFirst {
					t.Errorf("GetPackagesBelowThreshold() first = %q, want %q", deficits[0].PackagePath, tt.wantFirst)
				}

				// 检查缺口计算
				if deficits[0].Deficit <= 0 {
					t.Errorf("GetPackagesBelowThreshold() deficit = %.2f, want > 0", deficits[0].Deficit)
				}

				// 检查估算测试数
				if deficits[0].EstimatedTests <= 0 {
					t.Errorf("GetPackagesBelowThreshold() estimated tests = %d, want > 0", deficits[0].EstimatedTests)
				}
			}
		})
	}
}

func TestCoverageAnalyzer_CalculateRemediationPlan(t *testing.T) {
	tests := []struct {
		name               string
		threshold          float64
		report             *CoverageReport
		wantTotalTests     int
		wantActionCount    int
		wantHighPriority   bool
		wantMediumPriority bool
	}{
		{
			name:      "Coverage above threshold - no remediation needed",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage: 85.0,
				CoverageDeficit: 0,
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 85,
					},
				},
			},
			wantTotalTests:  0,
			wantActionCount: 0,
		},
		{
			name:      "Coverage below threshold - with handler and service",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage: 70.0,
				CoverageDeficit: 10.0,
				FileDetails: map[string]*FileDetail{
					"internal/handler/auth.go": {
						FilePath:          "internal/handler/auth.go",
						TotalStatements:   100,
						CoveredStatements: 70,
					},
					"internal/service/user.go": {
						FilePath:          "internal/service/user.go",
						TotalStatements:   100,
						CoveredStatements: 65,
					},
				},
			},
			wantActionCount:    2,
			wantHighPriority:   false, // handler 和 service/user 都不是高优先级
			wantMediumPriority: true,  // handler 应该是中优先级
		},
		{
			name:      "Large deficit requiring many tests",
			threshold: 80.0,
			report: &CoverageReport{
				OverallCoverage: 50.0,
				CoverageDeficit: 30.0,
				FileDetails: map[string]*FileDetail{
					"internal/service/auth.go": {
						FilePath:          "internal/service/auth.go",
						TotalStatements:   500,
						CoveredStatements: 250,
					},
				},
			},
			wantActionCount: 1,
			// 250个未覆盖语句，估算需要约 36 个测试 (250/7)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewCoverageAnalyzer(tt.threshold, nil)
			plan := analyzer.CalculateRemediationPlan(tt.report)

			if plan.CurrentCoverage != tt.report.OverallCoverage {
				t.Errorf("CalculateRemediationPlan() current coverage = %.2f, want %.2f",
					plan.CurrentCoverage, tt.report.OverallCoverage)
			}

			if plan.TargetCoverage != tt.threshold {
				t.Errorf("CalculateRemediationPlan() target coverage = %.2f, want %.2f",
					plan.TargetCoverage, tt.threshold)
			}

			if len(plan.PackageActions) != tt.wantActionCount {
				t.Errorf("CalculateRemediationPlan() action count = %d, want %d",
					len(plan.PackageActions), tt.wantActionCount)
			}

			if tt.wantTotalTests > 0 && plan.TotalEstimatedTests <= 0 {
				t.Errorf("CalculateRemediationPlan() total tests = %d, want > 0",
					plan.TotalEstimatedTests)
			}

			// 检查优先级
			if tt.wantActionCount > 0 {
				hasHigh := false
				hasMedium := false
				for _, action := range plan.PackageActions {
					if action.Priority == "High" {
						hasHigh = true
					}
					if action.Priority == "Medium" {
						hasMedium = true
					}

					// 检查每个行动都有建议
					if len(action.Suggestions) == 0 {
						t.Errorf("PackageAction for %s has no suggestions", action.PackagePath)
					}
				}

				if tt.wantHighPriority && !hasHigh {
					t.Error("CalculateRemediationPlan() expected high priority action but got none")
				}
				if tt.wantMediumPriority && !hasMedium {
					t.Error("CalculateRemediationPlan() expected medium priority action but got none")
				}
			}
		})
	}
}

func TestGetPackagePriority(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, []string{
		"internal/service/auth.go",
		"internal/service/mfa.go",
		"internal/handler/auth.go",
	})

	tests := []struct {
		name        string
		packagePath string
		want        string
	}{
		{
			name:        "Auth service - high priority",
			packagePath: "internal/service/auth",
			want:        "High",
		},
		{
			name:        "MFA service - high priority",
			packagePath: "internal/service/mfa",
			want:        "High",
		},
		{
			name:        "User service - medium priority",
			packagePath: "internal/service/user",
			want:        "Medium",
		},
		{
			name:        "Handler - medium priority",
			packagePath: "internal/handler",
			want:        "Medium",
		},
		{
			name:        "Util - low priority",
			packagePath: "internal/util",
			want:        "Low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzer.getPackagePriority(tt.packagePath)
			if got != tt.want {
				t.Errorf("getPackagePriority(%q) = %q, want %q", tt.packagePath, got, tt.want)
			}
		})
	}
}

func TestGenerateSuggestions(t *testing.T) {
	analyzer := NewCoverageAnalyzer(80.0, nil)

	tests := []struct {
		name         string
		deficit      PackageDeficit
		wantContains []string
	}{
		{
			name: "Handler package",
			deficit: PackageDeficit{
				PackagePath:     "internal/handler",
				CurrentCoverage: 70.0,
				Deficit:         10.0,
				UncoveredStmt:   50,
				EstimatedTests:  7,
			},
			wantContains: []string{
				"HTTP handler test",
				"table-driven tests",
				"request validation",
			},
		},
		{
			name: "Service package",
			deficit: PackageDeficit{
				PackagePath:     "internal/service",
				CurrentCoverage: 65.0,
				Deficit:         15.0,
				UncoveredStmt:   70,
				EstimatedTests:  10,
			},
			wantContains: []string{
				"business logic",
				"error conditions",
				"Mock Store",
			},
		},
		{
			name: "Store package",
			deficit: PackageDeficit{
				PackagePath:     "internal/store/postgres",
				CurrentCoverage: 60.0,
				Deficit:         20.0,
				UncoveredStmt:   100,
				EstimatedTests:  14,
			},
			wantContains: []string{
				"database integration",
				"CRUD operations",
				"transaction rollback",
			},
		},
		{
			name: "High deficit warning",
			deficit: PackageDeficit{
				PackagePath:     "internal/service",
				CurrentCoverage: 50.0,
				Deficit:         30.0,
				UncoveredStmt:   200,
				EstimatedTests:  28,
			},
			wantContains: []string{
				"⚠️",
				"High deficit",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := analyzer.generateSuggestions(tt.deficit)

			if len(suggestions) == 0 {
				t.Error("generateSuggestions() returned no suggestions")
			}

			// 检查是否包含预期的关键词
			allSuggestions := ""
			for _, s := range suggestions {
				allSuggestions += s + " "
			}

			for _, want := range tt.wantContains {
				found := false
				for _, s := range suggestions {
					if contains := stringContains(s, want); contains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("generateSuggestions() missing expected keyword: %q\nGot: %v", want, suggestions)
				}
			}
		})
	}
}

// stringContains 检查字符串是否包含子串（忽略大小写）
func stringContains(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)

	if len(substr) > len(s) {
		return false
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// toLower 转换为小写
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		result[i] = c
	}
	return string(result)
}
