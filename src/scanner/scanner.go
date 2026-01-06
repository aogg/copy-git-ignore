package scanner

import (
	"fmt"
	"os"
	"path/filepath"
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
		// 第一步：检查仓库根目录下的直接子目录是否被忽略
		// 这样可以一次性识别出整个被忽略的目录（如 demo/）
		ignoredDirs := make(map[string]bool)
		directIgnoredDirs := make(map[string]bool)

		// 读取仓库根目录
		rootEntries, err := os.ReadDir(repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 读取仓库目录 %s 失败: %v\n", repoRoot, err)
			continue
		}

		// 检查每个直接子目录是否被忽略
		for _, entry := range rootEntries {
			if !entry.IsDir() {
				continue // 只处理目录
			}

			dirName := entry.Name()
			dirPath := filepath.Join(repoRoot, dirName)

			// 应用排除规则
			if excluder.ShouldExclude(dirPath) {
				continue
			}

			// 检查目录是否被忽略
			isIgnored, err := git.IsPathIgnored(repoRoot, dirPath)
			if err != nil {
				// 检查失败，跳过这个目录
				continue
			}

			if isIgnored {
				// 目录被忽略，标记它和其所有子目录
				directIgnoredDirs[dirPath] = true
				ignoredDirs[dirPath] = true

				// 计算相对于搜索根目录的相对路径
				relToSearchRoot, err := filepath.Rel(searchRoot, dirPath)
				if err != nil {
					relToSearchRoot = dirPath
				}

				// 添加目录到结果
				dirInfo := IgnoredFileInfo{
					AbsPath:      dirPath,
					RelativePath: relToSearchRoot,
					RepoRoot:     repoRoot,
				}
				allFiles = append(allFiles, dirInfo)
			}
		}

		// 第二步：获取被忽略的文件列表
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

			// 检查文件是否在任何被忽略的直接子目录下
			// 如果在，直接跳过这个文件，不需要再检查其父目录
			skipFile := false
			for ignoredDir := range directIgnoredDirs {
				prefix := ignoredDir + string(filepath.Separator)
				if strings.HasPrefix(absPath, prefix) || absPath == ignoredDir {
					skipFile = true
					break
				}
			}
			if skipFile {
				continue
			}

			// 检查文件的其他父目录是否被忽略（不包括直接子目录）
			// 从文件的直接父目录开始，向上检查到仓库根目录
			currentPath := filepath.Dir(absPath)
			for {
				if currentPath == repoRoot {
					break
				}

				// 如果这个目录已经被标记为被忽略，跳过当前文件
				if ignoredDirs[currentPath] {
					skipFile = true
					break
				}

				// 如果这个目录是被忽略的直接子目录，跳过当前文件
				if directIgnoredDirs[currentPath] {
					skipFile = true
					break
				}

				// 检查这个目录是否被忽略
				isIgnored, err := git.IsPathIgnored(repoRoot, currentPath)
				if err != nil {
					// 检查失败，继续处理文件
					currentPath = filepath.Dir(currentPath)
					continue
				}

				if isIgnored {
					// 目录被忽略，标记并跳过所有该目录下的文件
					ignoredDirs[currentPath] = true
					skipFile = true

					// 计算相对于搜索根目录的相对路径
					relToSearchRoot, err := filepath.Rel(searchRoot, currentPath)
					if err != nil {
						relToSearchRoot = currentPath
					}

					// 添加目录到结果
					dirInfo := IgnoredFileInfo{
						AbsPath:      currentPath,
						RelativePath: relToSearchRoot,
						RepoRoot:     repoRoot,
					}
					repoFiles = append(repoFiles, dirInfo)
					break
				}

				// 继续检查父目录
				currentPath = filepath.Dir(currentPath)
			}

			// 如果文件应该被跳过，继续处理下一个文件
			if skipFile {
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
	filteredFiles := FilterRedundantFiles(repoFiles, ignoredDirs)
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
// 如果一个文件夹下的多个文件都被忽略，则用文件夹路径替换所有子文件路径
// ignoredDirs: 已经被标记为被忽略的目录（这些目录不需要再进行聚合优化）
func FilterRedundantFiles(files []IgnoredFileInfo, ignoredDirs map[string]bool) []IgnoredFileInfo {
	if len(files) == 0 {
		return files
	}

	// 按仓库分组处理
	repoGroups := make(map[string][]IgnoredFileInfo)
	for _, file := range files {
		repoGroups[file.RepoRoot] = append(repoGroups[file.RepoRoot], file)
	}

	var result []IgnoredFileInfo

	for repoRoot, repoFiles := range repoGroups {
		// 统计每个目录下的文件数量（相对于仓库根目录）
		dirFileCount := make(map[string]int)
		dirFiles := make(map[string][]IgnoredFileInfo)

		for _, file := range repoFiles {
			// 计算相对于仓库根目录的路径
			relToRepo, err := filepath.Rel(repoRoot, file.AbsPath)
			if err != nil {
				continue
			}

			dir := filepath.Dir(relToRepo)
			if dir == "." {
				dir = ""
			}
			dirFileCount[dir]++
			dirFiles[dir] = append(dirFiles[dir], file)
		}

		// 找出需要替换为目录的路径
		dirsToReplace := make(map[string]bool)

		for dir, count := range dirFileCount {
			// 跳过已经被标记为被忽略的目录（这些目录已经作为独立条目）
			dirAbsPath := filepath.Join(repoRoot, dir)
			if ignoredDirs[dirAbsPath] {
				continue
			}

			if count >= 2 {
				dirsToReplace[dir] = true
			}
		}

		// 生成结果
		for dir := range dirsToReplace {
			if dir == "" {
				// 仓库根目录
				searchRoot := filepath.Dir(repoRoot)
				relToSearchRoot, err := filepath.Rel(searchRoot, repoRoot)
				if err != nil {
					relToSearchRoot = filepath.Base(repoRoot)
				}

				dirInfo := IgnoredFileInfo{
					AbsPath:      repoRoot,
					RelativePath: strings.ReplaceAll(relToSearchRoot, "/", string(filepath.Separator)),
					RepoRoot:     repoRoot,
				}
				result = append(result, dirInfo)
			} else {
				// 子目录
				dirAbsPath := filepath.Join(repoRoot, dir)
				searchRoot := filepath.Dir(repoRoot)
				repoRel, err := filepath.Rel(searchRoot, repoRoot)
				if err != nil {
					continue
				}
				relToSearchRoot := filepath.Join(repoRel, dir)

				dirInfo := IgnoredFileInfo{
					AbsPath:      dirAbsPath,
					RelativePath: strings.ReplaceAll(relToSearchRoot, "/", string(filepath.Separator)),
					RepoRoot:     repoRoot,
				}
				result = append(result, dirInfo)
			}
		}

		// 添加不需要替换的文件（单个文件或不满足替换条件的目录下的文件）
		for dir, fileList := range dirFiles {
			if !dirsToReplace[dir] {
				result = append(result, fileList...)
			}
		}
	}

	return result
}
