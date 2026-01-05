package tests

import (
	"testing"

	"github.com/aogg/copy-ignore/src/exclude"
)

func TestExcludeMatcher(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		expected bool
	}{
		// 绝对路径排除测试
		{
			name:     "绝对路径完全匹配",
			patterns: []string{"C:/temp"},
			path:     "C:\\temp\\file.txt",
			expected: true,
		},
		{
			name:     "绝对路径前缀匹配",
			patterns: []string{"C:/temp"},
			path:     "C:\\temp\\subdir\\file.txt",
			expected: true,
		},
		{
			name:     "绝对路径不匹配",
			patterns: []string{"C:/temp"},
			path:     "D:\\temp\\file.txt",
			expected: false,
		},
		// glob 模式测试
		{
			name:     "glob 星号匹配",
			patterns: []string{"*.log"},
			path:     "C:\\project\\debug.log",
			expected: true,
		},
		{
			name:     "glob 星号不匹配",
			patterns: []string{"*.log"},
			path:     "C:\\project\\debug.txt",
			expected: false,
		},
		{
			name:     "glob 双星号匹配",
			patterns: []string{"**/vendor/**"},
			path:     "C:/project/subdir/vendor/lib.a",
			expected: true,
		},
		{
			name:     "glob 双星号不匹配",
			patterns: []string{"**/vendor/**"},
			path:     "C:\\project\\src\\lib.a",
			expected: false,
		},
		// 混合测试
		{
			name:     "绝对路径和 glob 混合",
			patterns: []string{"C:/temp", "*.log"},
			path:     "C:\\project\\debug.log",
			expected: true,
		},
		{
			name:     "空模式列表",
			patterns: []string{},
			path:     "C:\\any\\path",
			expected: false,
		},
		{
			name:     "多个模式，任一匹配",
			patterns: []string{"*.txt", "*.log"},
			path:     "C:\\file.log",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := exclude.NewMatcher(tt.patterns)
			if err != nil {
				t.Fatalf("创建匹配器失败: %v", err)
			}

			result := matcher.ShouldExclude(tt.path)
			if result != tt.expected {
				t.Errorf("期望 %v，得到 %v，路径: %s，模式: %v",
					tt.expected, result, tt.path, tt.patterns)
			}
		})
	}
}

func TestExcludeMatcher_WindowsPaths(t *testing.T) {
	// 在 Windows 上测试路径归一化
	tests := []struct {
		name     string
		patterns []string
		path     string
		expected bool
	}{
		{
			name:     "反斜杠和正斜杠混合",
			patterns: []string{"C:/temp\\subdir"},
			path:     "C:\\temp\\subdir\\file.txt",
			expected: true,
		},
		{
			name:     "大小写不敏感",
			patterns: []string{"C:/TEMP"},
			path:     "c:\\temp\\file.txt",
			expected: true,
		},
		{
			name:     "相对路径 glob",
			patterns: []string{"**/vendor/**"},
			path:     "some/repo/vendor/lib.a",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := exclude.NewMatcher(tt.patterns)
			if err != nil {
				t.Fatalf("创建匹配器失败: %v", err)
			}

			result := matcher.ShouldExclude(tt.path)
			if result != tt.expected {
				t.Errorf("期望 %v，得到 %v，路径: %s，模式: %v",
					tt.expected, result, tt.path, tt.patterns)
			}
		})
	}
}

func TestExcludeMatcher_InvalidPatterns(t *testing.T) {
	// 测试无效模式（应该被跳过，不会导致错误）
	patterns := []string{"", "[invalid", "valid*.txt"}
	matcher, err := exclude.NewMatcher(patterns)
	if err != nil {
		t.Fatalf("创建匹配器失败: %v", err)
	}

	// 有效的模式应该仍然工作
	if !matcher.ShouldExclude("C:\\project\\validtest.txt") {
		t.Error("有效的模式应该匹配")
	}
}
