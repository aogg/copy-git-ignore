package exclude

import (
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Matcher 负责匹配排除模式
type Matcher struct {
	patterns []string
}

// Patterns 返回匹配器的模式列表（用于调试）
func (m *Matcher) Patterns() []string {
	return m.patterns
}

// NewMatcher 创建一个新的排除匹配器
func NewMatcher(patterns []string) (*Matcher, error) {
	m := &Matcher{
		patterns: make([]string, 0, len(patterns)),
	}

	// 预处理和验证模式
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

	// 转换为正斜杠格式（doublestar 需要），但不使用 filepath.Clean 以避免破坏通配符
	normalized := strings.ReplaceAll(pattern, "\\", "/")

	// 处理相对路径模式
	if !m.isAbsolutePathPattern(normalized) {
		// 检查是否包含通配符
		hasWildcard := strings.Contains(normalized, "*") || strings.Contains(normalized, "?") || strings.Contains(normalized, "[")
		if hasWildcard {
			// 对于包含通配符的模式，如果是简单的目录匹配模式（如 */vendor/*），转换为 **/vendor/**
			if m.isSimpleDirPattern(normalized) {
				// 提取目录名，如从 */vendor/* 提取 vendor
				dirName := m.extractDirFromPattern(normalized)
				if dirName != "" {
					normalized = "**/" + dirName + "/**"
				}
			} else if !strings.Contains(normalized, "/") {
				// 对于不包含路径分隔符的简单通配符模式（如 *.log），添加 **/ 前缀
				// 使其能在任何目录下匹配
				normalized = "**/" + normalized
			} else {
				// 对于包含路径分隔符的通配符模式（如 */*.log, dir/*.log），保持原样
				// 用户明确指定了目录结构，不自动添加 **/ 前缀
			}
		} else {
			// 对于不包含通配符的相对路径模式，添加 **/ 前缀和 /** 后缀，使其匹配任何路径中包含该目录的情况
			normalized = "**/" + normalized + "/**"
		}
	}

		m.patterns = append(m.patterns, normalized)
	}

	return m, nil
}

// ShouldExclude 检查指定路径是否应该被排除
func (m *Matcher) ShouldExclude(path string) bool {
	if len(m.patterns) == 0 {
		return false
	}

	// 归一化待检查的路径，并转换为正斜杠（doublestar 需要）
	cleanPath := filepath.Clean(path)
	normalizedPath := strings.ReplaceAll(cleanPath, "\\", "/")

	// 检查每个模式
	for _, pattern := range m.patterns {
		if m.matchesPattern(normalizedPath, pattern) {
			return true
		}
	}

	return false
}

// matchesPattern 检查单个模式是否匹配路径
func (m *Matcher) matchesPattern(path, pattern string) bool {
	// 检查是否为绝对路径模式
	if m.isAbsolutePathPattern(pattern) {
		// 对于绝对路径模式，使用前缀匹配
		return m.matchesAbsolutePath(path, pattern)
	}

	// 对于 glob 模式，使用 doublestar 匹配
	// path 已经转换为正斜杠格式
	matched, err := doublestar.Match(pattern, path)
	if err != nil {
		// 如果模式无效，跳过
		return false
	}
	return matched
}

// isAbsolutePathPattern 判断模式是否为绝对路径模式
func (m *Matcher) isAbsolutePathPattern(pattern string) bool {
	// Windows 绝对路径：以驱动器字母开头（如 C:/ 或 C:\）
	if len(pattern) >= 3 && pattern[1] == ':' && (pattern[2] == '/' || pattern[2] == '\\') {
		return true
	}

	// UNC 路径：以 // 或 \\ 开头
	if strings.HasPrefix(pattern, "//") || strings.HasPrefix(pattern, "\\\\") {
		return true
	}

	// 以 / 开头的 Unix 风格绝对路径（在 Windows 上可能也有效）
	if strings.HasPrefix(pattern, "/") {
		return true
	}

	return false
}

// matchesAbsolutePath 检查绝对路径模式是否匹配
func (m *Matcher) matchesAbsolutePath(path, pattern string) bool {
	// path 已经是正斜杠格式，pattern 可能是反斜杠格式
	// 将 pattern 也转换为正斜杠格式以便比较
	normalizedPattern := strings.ReplaceAll(pattern, "\\", "/")

	// 在 Windows 上，路径比较不区分大小写
	pathLower := strings.ToLower(path)
	patternLower := strings.ToLower(normalizedPattern)

	// 检查路径是否以前缀模式开头
	return strings.HasPrefix(pathLower, patternLower)
}

// isSimpleDirPattern 检查是否为简单的目录匹配模式（如 */vendor/* 或 vendor）
func (m *Matcher) isSimpleDirPattern(pattern string) bool {
	// 检查模式是否为 */dirname/* 或 */dirname 格式
	if strings.HasPrefix(pattern, "*/") {
		remaining := strings.TrimPrefix(pattern, "*/")
		if strings.HasSuffix(remaining, "/*") {
			dirName := strings.TrimSuffix(remaining, "/*")
			return dirName != "" && !strings.Contains(dirName, "*") && !strings.Contains(dirName, "?") && !strings.Contains(dirName, "[")
		}
		if !strings.Contains(remaining, "*") && !strings.Contains(remaining, "?") && !strings.Contains(remaining, "[") {
			return remaining != ""
		}
	}
	return false
}

// extractDirFromPattern 从简单目录模式中提取目录名
func (m *Matcher) extractDirFromPattern(pattern string) string {
	if strings.HasPrefix(pattern, "*/") {
		remaining := strings.TrimPrefix(pattern, "*/")
		if strings.HasSuffix(remaining, "/*") {
			return strings.TrimSuffix(remaining, "/*")
		}
		return remaining
	}
	return ""
}
