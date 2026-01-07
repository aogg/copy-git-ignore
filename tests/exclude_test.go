package tests

import (
	"fmt"
	"testing"

	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/bmatcuk/doublestar/v4"
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
		{
			name:     "星号斜杠星号模式匹配单层子目录文件",
			patterns: []string{"*/*.log"},
			path:     "project\\debug.log",
			expected: true,
		},
		{
			name:     "星号斜杠星号模式不匹配根目录文件",
			patterns: []string{"*/*.log"},
			path:     "debug.log",
			expected: false,
		},
		{
			name:     "星号斜杠星号模式不匹配深层目录文件",
			patterns: []string{"*/*.log"},
			path:     "project\\subdir\\debug.log",
			expected: false,
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
				t.Errorf("期望 %v，得到 %v，路径: %s，模式: %v, 处理后模式: %v",
					tt.expected, result, tt.path, tt.patterns, matcher.Patterns())
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

func TestDebugDoublestar(t *testing.T) {
	// 调试 doublestar 匹配行为
	testCases := []struct {
		pattern, path string
		expected      bool
	}{
		{"*", "file.log", true},
		{"*.log", "file.log", true},
		{"*/*.log", "dir/file.log", true},
		{"*/*.log", "file.log", false},
		{"*/*.log", "dir/subdir/file.log", false},
		{"*/*.log", "project/debug.log", false},
		{"*/*.log", "project/subdir/debug.log", true},
	}

	for _, tc := range testCases {
		matched, err := doublestar.Match(tc.pattern, tc.path)
		if err != nil {
			t.Errorf("模式 %s 匹配路径 %s 时出错: %v", tc.pattern, tc.path, err)
			continue
		}
		if matched != tc.expected {
			t.Errorf("模式 %s 匹配路径 %s: 期望 %v, 得到 %v", tc.pattern, tc.path, tc.expected, matched)
		} else {
			fmt.Printf("✓ 模式 %s 匹配路径 %s: %v\n", tc.pattern, tc.path, matched)
		}
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
