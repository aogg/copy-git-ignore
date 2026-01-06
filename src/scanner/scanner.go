package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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
	return ScanIgnoredFilesWithProgress(searchRoot, excluder, nil)
}

// ScanIgnoredFilesWithProgress 扫描指定根目录下的所有 Git 仓库，并返回所有被忽略且未被排除的文件
// progress 回调函数会在扫描过程中被调用，传入当前正在扫描的绝对路径
func ScanIgnoredFilesWithProgress(searchRoot string, excluder interface{ ShouldExclude(path string) bool }, progress func(absPath string)) ([]IgnoredFileInfo, error) {
	var allFiles []IgnoredFileInfo

	// 递归查找所有 Git 仓库
	repos, err := findGitRepositoriesWithProgress(searchRoot, progress)
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
		directIgnoredDirs := make(map[string]bool)

		// 读取仓库根目录
		rootEntries, err := os.ReadDir(repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 读取仓库目录 %s 失败: %v\n", repoRoot, err)
			continue
		}

		// 检查每个直接子目录是否被忽略（只检查直接子目录，一次性批量处理）
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
				directIgnoredDirs[dirPath] = true

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

		// 过滤掉被父目录包含的文件（聚合优化）
		ignoredDirs := make(map[string]bool)
		for dir := range directIgnoredDirs {
			ignoredDirs[dir] = true
		}
		filteredFiles := FilterRedundantFiles(repoFiles, ignoredDirs)
		allFiles = append(allFiles, filteredFiles...)
	}

	return allFiles, nil
}

// ScanIgnoredFilesWithProgressStream 扫描指定根目录下的所有 Git 仓库，
// 将发现的文件实时发送到fileChan，支持进度回调
func ScanIgnoredFilesWithProgressStream(searchRoot string, excluder interface{ ShouldExclude(path string) bool }, progress func(absPath string), fileChan chan<- IgnoredFileInfo) error {
	return ScanIgnoredFilesWithProgressStreamConcurrent(searchRoot, excluder, progress, fileChan, runtime.NumCPU())
}

// ScanIgnoredFilesWithProgressStreamConcurrent 并发扫描指定根目录下的所有 Git 仓库，
// 将发现的文件实时发送到fileChan，支持进度回调和并发处理
func ScanIgnoredFilesWithProgressStreamConcurrent(searchRoot string, excluder interface{ ShouldExclude(path string) bool }, progress func(absPath string), fileChan chan<- IgnoredFileInfo, numWorkers int) error {
	ctx := context.Background()

	// 创建任务通道，缓冲大小为 numWorkers*2 以减少阻塞
	jobs := make(chan string, numWorkers*2)
	var wg sync.WaitGroup

	// 启动 worker goroutines
	for i := 0; i < numWorkers; i++ {
		go func() {
			for {
				select {
				case repoRoot, ok := <-jobs:
					if !ok {
						return // 通道已关闭，退出
					}
					processRepository(ctx, repoRoot, searchRoot, excluder, fileChan)
					wg.Done()
				case <-ctx.Done():
					return // 上下文取消，退出
				}
			}
		}()
	}

	fmt.Println()
	fmt.Println("开始扫描 Git 仓库")
	// 开始时间
	startTime := time.Now()
	fmt.Printf("开始时间: %s\n", startTime.Format("2006-01-02 15:04:05.000"))
	fmt.Printf("搜索根目录: %s\n", searchRoot)
	fmt.Printf("排除规则: %v\n", excluder)
	fmt.Println()

	// 使用队列实现广度优先搜索，同时在发现仓库时应用排除规则
	queue := []string{searchRoot}
	visited := make(map[string]bool)
	repoCount := 0

	for len(queue) > 0 {
		currentDir := queue[0]
		queue = queue[1:]

		// 避免重复处理
		if visited[currentDir] {
			continue
		}
		visited[currentDir] = true

		// 调用进度回调
		if progress != nil {
			progress(currentDir)
		}

		// 先判断当前目录是否为 Git 仓库
		if isGitRepo(currentDir) {
			// 应用排除规则到仓库根目录
			if !excluder.ShouldExclude(currentDir) {
				repoCount++
				wg.Add(1)
				jobs <- currentDir
			}
			// 如果是 Git 仓库，后续就不需要扫描这个文件夹的子孙了
			continue
		}

		// 如果不是 Git 仓库，才扫描其子目录
		entries, err := os.ReadDir(currentDir)
		if err != nil {
			// 跳过无法访问的目录
			if os.IsPermission(err) {
				continue
			}
			return err
		}

		// 将子目录添加到队列中（广度优先）
		for _, entry := range entries {
			if entry.IsDir() {
				childDir := filepath.Join(currentDir, entry.Name())
				// 确保不超出搜索根目录
				if rel, err := filepath.Rel(searchRoot, childDir); err == nil && !strings.HasPrefix(rel, "..") {
					queue = append(queue, childDir)
				}
			}
		}
	}

	// 输出详细
	fmt.Println()
	fmt.Printf("Git 仓库数量: %d\n", repoCount)

	if repoCount > 0 {
		fmt.Println()
		fmt.Println()
		fmt.Println("开始并发扫描 Git 仓库")
	}

	// 关闭任务通道，表示不再发送新任务
	close(jobs)

	// 等待所有仓库处理完成
	wg.Wait()

	fmt.Println()
	fmt.Println("所有仓库处理完成")
	fmt.Printf("扫描结束时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	return nil
}

// processRepository 处理单个 Git 仓库，获取被忽略的文件并发送到 fileChan
func processRepository(ctx context.Context, repoRoot, searchRoot string, excluder interface{ ShouldExclude(path string) bool }, fileChan chan<- IgnoredFileInfo) {
	startTime := time.Now()
	fileCount := 0
	var processError error

	defer func() {
		endTime := time.Now()
		duration := endTime.Sub(startTime)

		// 处理完成后立即输出结果
		if processError == nil {
			fmt.Printf("✓ 仓库: %s\n", repoRoot)
			fmt.Printf("  开始时间: %s\n", startTime.Format("2006-01-02 15:04:05.000"))
			fmt.Printf("  结束时间: %s\n", endTime.Format("2006-01-02 15:04:05.000"))
			fmt.Printf("  处理耗时: %v\n", duration)
			fmt.Printf("  发现文件: %d 个\n", fileCount)
		} else {
			fmt.Printf("✗ 仓库: %s\n", repoRoot)
			fmt.Printf("  开始时间: %s\n", startTime.Format("2006-01-02 15:04:05.000"))
			fmt.Printf("  结束时间: %s\n", endTime.Format("2006-01-02 15:04:05.000"))
			fmt.Printf("  处理耗时: %v\n", duration)
			fmt.Printf("  错误: %v\n", processError)
		}
		fmt.Println()
	}()

	// 第一步：检查仓库根目录下的直接子目录是否被忽略
	directIgnoredDirs := make(map[string]bool)

	// 读取仓库根目录
	rootEntries, err := os.ReadDir(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 读取仓库目录 %s 失败: %v\n", repoRoot, err)
		processError = err
		return
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
			continue
		}

		if isIgnored {
			directIgnoredDirs[dirPath] = true

			// 计算相对于搜索根目录的相对路径
			relToSearchRoot, err := filepath.Rel(searchRoot, dirPath)
			if err != nil {
				relToSearchRoot = dirPath
			}

			// 立即发送到复制channel
			dirInfo := IgnoredFileInfo{
				AbsPath:      dirPath,
				RelativePath: relToSearchRoot,
				RepoRoot:     repoRoot,
			}
			select {
			case fileChan <- dirInfo:
				fileCount++
			case <-ctx.Done():
				return
			}
		}
	}

	// 第二步：获取被忽略的文件列表
	files, err := git.ListIgnoredFiles(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 处理仓库 %s 时出错: %v\n", repoRoot, err)
		processError = err
		return
	}

	// 处理每个被忽略的文件
	for _, relPath := range files {
		absPath := filepath.Join(repoRoot, relPath)

		// 应用排除规则
		if excluder.ShouldExclude(absPath) {
			continue
		}

		// 检查文件是否在任何被忽略的直接子目录下
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

		// 计算相对于搜索根目录的相对路径
		relToSearchRoot, err := filepath.Rel(searchRoot, absPath)
		if err != nil {
			relToSearchRoot = absPath
		}

		fileInfo := IgnoredFileInfo{
			AbsPath:      absPath,
			RelativePath: relToSearchRoot,
			RepoRoot:     repoRoot,
		}

		// 立即发送到复制channel
		select {
		case fileChan <- fileInfo:
			fileCount++
		case <-ctx.Done():
			return
		}
	}
}

// findGitRepositories 递归查找指定目录下的所有 Git 仓库
// 返回所有找到的仓库根目录列表
func findGitRepositories(root string) ([]string, error) {
	return findGitRepositoriesWithProgress(root, nil)
}

// findGitRepositoriesWithProgress 广度优先查找指定目录下的所有 Git 仓库
// progress 回调函数会在遍历过程中被调用，传入当前正在扫描的绝对路径
// 返回所有找到的仓库根目录列表
func findGitRepositoriesWithProgress(root string, progress func(absPath string)) ([]string, error) {
	var repos []string

	// 使用队列实现广度优先搜索
	queue := []string{root}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		currentDir := queue[0]
		queue = queue[1:]

		// 避免重复处理
		if visited[currentDir] {
			continue
		}
		visited[currentDir] = true

		// 调用进度回调
		if progress != nil {
			progress(currentDir)
		}

		// 先判断当前目录是否为 Git 仓库
		if isGitRepo(currentDir) {
			repos = append(repos, currentDir)
			// 如果是 Git 仓库，后续就不需要扫描这个文件夹的子孙了
			continue
		}

		// 如果不是 Git 仓库，才扫描其子目录
		entries, err := os.ReadDir(currentDir)
		if err != nil {
			// 跳过无法访问的目录
			if os.IsPermission(err) {
				continue
			}
			return nil, err
		}

		// 将子目录添加到队列中（广度优先）
		for _, entry := range entries {
			if entry.IsDir() {
				childDir := filepath.Join(currentDir, entry.Name())
				// 确保不超出搜索根目录
				if rel, err := filepath.Rel(root, childDir); err == nil && !strings.HasPrefix(rel, "..") {
					queue = append(queue, childDir)
				}
			}
		}
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
