package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aogg/copy-ignore/src/git"
)

// IgnoredFileInfo 表示一个被忽略的文件信息
type IgnoredFileInfo struct {
	AbsPath      string // 文件的绝对路径
	RelativePath string // 相对于搜索根目录的相对路径
	RepoRoot     string // 文件所属的 Git 仓库根目录
}

// ScanIgnoredFiles 扫描指定根目录下的所有 Git 仓库，并返回所有被忽略且未被排除的文件
func ScanIgnoredFiles(searchRoot string, excluder interface{ ShouldExclude(path string) bool }) ([]IgnoredFileInfo, error) {
	var allFiles []IgnoredFileInfo

	// 递归查找所有 Git 仓库
	repos, err := findGitRepositories(searchRoot)
	if err != nil {
		return nil, fmt.Errorf("查找 Git 仓库失败: %v", err)
	}

	if len(repos) == 0 {
		return allFiles, nil
	}

	// 对每个仓库，获取被忽略的文件列表
	for _, repoRoot := range repos {
		files, err := git.ListIgnoredFiles(repoRoot)
		if err != nil {
			// 如果某个仓库失败，继续处理其他仓库，但记录警告
			fmt.Fprintf(os.Stderr, "警告: 处理仓库 %s 时出错: %v\n", repoRoot, err)
			continue
		}

		// 收集所有被忽略且未被排除的文件
		var repoFiles []IgnoredFileInfo
		for _, relPath := range files {
			absPath := filepath.Join(repoRoot, relPath)

			// 应用排除规则
			if excluder.ShouldExclude(absPath) {
				continue
			}

			// 计算相对于搜索根目录的相对路径
			relToSearchRoot, err := filepath.Rel(searchRoot, absPath)
			if err != nil {
				// 如果计算相对路径失败，使用绝对路径作为相对路径
				relToSearchRoot = absPath
			}

			fileInfo := IgnoredFileInfo{
				AbsPath:      absPath,
				RelativePath: relToSearchRoot,
				RepoRoot:     repoRoot,
			}

			repoFiles = append(repoFiles, fileInfo)
		}

		// 过滤掉被父目录包含的文件
		filteredFiles := FilterRedundantFiles(repoFiles)
		allFiles = append(allFiles, filteredFiles...)
	}

	return allFiles, nil
}

// findGitRepositories 递归查找指定目录下的所有 Git 仓库
// 返回所有找到的仓库根目录列表
func findGitRepositories(root string) ([]string, error) {
	var repos []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// 跳过无法访问的目录
			if os.IsPermission(err) {
				return filepath.SkipDir
			}
			return err
		}

		// 只处理目录
		if !info.IsDir() {
			return nil
		}

		// 检查是否为 Git 仓库
		if isGitRepo(path) {
			repos = append(repos, path)

			// 找到仓库后，继续扫描其子目录（可能有嵌套仓库）
			// 每个 .git 目录都是独立的仓库
			return nil
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return repos, nil
}

// isGitRepo 检查指定目录是否为 Git 仓库
func isGitRepo(dir string) bool {
	// 检查 .git 目录是否存在
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true
	}

	// 也检查 .git 文件（用于 git worktree）
	if gitFile := filepath.Join(dir, ".git"); func() bool {
		content, err := os.ReadFile(gitFile)
		if err != nil {
			return false
		}
		// 如果 .git 文件指向另一个目录，则可能是 worktree
		line := strings.TrimSpace(string(content))
		if strings.HasPrefix(line, "gitdir: ") {
			gitDirPath := strings.TrimPrefix(line, "gitdir: ")
			if _, err := os.Stat(filepath.Join(dir, gitDirPath)); err == nil {
				return true
			}
		}
		return false
	}() {
		return true
	}

	return false
}

// FilterRedundantFiles 过滤掉被父目录包含的文件
// 如果一个文件夹需要被复制，那么它的所有子文件和子文件夹都不需要单独列出
func FilterRedundantFiles(files []IgnoredFileInfo) []IgnoredFileInfo {
	if len(files) == 0 {
		return files
	}

	// 将文件按路径排序，确保父目录在子文件之前
	sort.Slice(files, func(i, j int) bool {
		pathI := strings.ReplaceAll(files[i].RelativePath, "\\", "/")
		pathJ := strings.ReplaceAll(files[j].RelativePath, "\\", "/")
		return pathI < pathJ
	})

	var result []IgnoredFileInfo
	parentDirs := make(map[string]bool) // 记录已包含的目录路径

	for _, file := range files {
		relPath := strings.ReplaceAll(file.RelativePath, "\\", "/")

		// 检查是否有父目录已经被包含
		isRedundant := false
		parts := strings.Split(relPath, "/")

		// 逐级检查父目录
		for i := 1; i < len(parts); i++ {
			parentPath := strings.Join(parts[:i], "/")
			if parentDirs[parentPath] {
				isRedundant = true
				break
			}
		}

		if !isRedundant {
			result = append(result, file)

			// 如果这是一个目录，将其标记为已包含
			if info, err := os.Stat(file.AbsPath); err == nil && info.IsDir() {
				parentDirs[relPath] = true
			}
		}
	}

	return result
}
