package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/aogg/copy-ignore/src/scanner"
)

func TestScanIgnoredFiles_NoGitRepos(t *testing.T) {
	// 测试没有 Git 仓库的情况
	tempDir := t.TempDir()

	excluder, err := exclude.NewMatcher([]string{})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	files, err := scanner.ScanIgnoredFiles(tempDir, excluder)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("期望找到 0 个文件，实际找到 %d 个", len(files))
	}
}

func TestScanIgnoredFiles_WithGitRepo(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git 不在 PATH 中，跳过测试")
	}

	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")

	// 创建并初始化 Git 仓库
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	initGitRepo(t, repoDir)

	// 创建 .gitignore 和被忽略的文件
	createGitignore(t, repoDir, "*.log\ntemp/\n")
	createIgnoredFilesInRepo(t, repoDir)

	excluder, err := exclude.NewMatcher([]string{})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	files, err := scanner.ScanIgnoredFiles(tempDir, excluder)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("期望找到被忽略的文件")
	}

	// 验证所有文件都来自正确的仓库
	for _, file := range files {
		if file.RepoRoot != repoDir {
			t.Errorf("文件应该来自仓库 %s，实际来自 %s", repoDir, file.RepoRoot)
		}

		// 验证绝对路径存在
		if _, err := os.Stat(file.AbsPath); os.IsNotExist(err) {
			t.Errorf("文件不存在: %s", file.AbsPath)
		}
	}
}

func TestScanIgnoredFiles_NestedRepos(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git 不在 PATH 中，跳过测试")
	}

	tempDir := t.TempDir()
	parentRepo := filepath.Join(tempDir, "parent")
	childRepo := filepath.Join(parentRepo, "child")

	// 创建父仓库
	if err := os.MkdirAll(parentRepo, 0755); err != nil {
		t.Fatalf("创建父目录失败: %v", err)
	}
	initGitRepo(t, parentRepo)
	createGitignore(t, parentRepo, "*.parent\n")
	createIgnoredFile(t, parentRepo, "file.parent", "parent content")

	// 创建子仓库（但由于父仓库的存在，子仓库不会被扫描到）
	if err := os.MkdirAll(childRepo, 0755); err != nil {
		t.Fatalf("创建子目录失败: %v", err)
	}
	initGitRepo(t, childRepo)
	createGitignore(t, childRepo, "*.child\n")
	createIgnoredFile(t, childRepo, "file.child", "child content")

	excluder, err := exclude.NewMatcher([]string{})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	files, err := scanner.ScanIgnoredFiles(tempDir, excluder)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}

	// 由于广度优先搜索且遇到git仓库就不再扫描子孙，现在应该只找到父仓库的文件
	if len(files) != 1 {
		t.Fatalf("期望找到 1 个文件（只有父仓库的），实际找到 %d 个", len(files))
	}

	// 验证只找到父仓库的文件
	file := files[0]
	if filepath.Ext(file.AbsPath) != ".parent" {
		t.Errorf("期望找到 .parent 文件，实际找到: %s", filepath.Ext(file.AbsPath))
	}
	if file.RepoRoot != parentRepo {
		t.Errorf("文件应该来自父仓库 %s，实际来自 %s", parentRepo, file.RepoRoot)
	}
}

func TestScanIgnoredFiles_WithExcludes(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git 不在 PATH 中，跳过测试")
	}

	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")

	// 创建并初始化 Git 仓库
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	initGitRepo(t, repoDir)
	createGitignore(t, repoDir, "*.log\n*.tmp\n")
	createIgnoredFilesInRepo(t, repoDir)

	// 使用排除模式
	excluder, err := exclude.NewMatcher([]string{"*.log"})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	files, err := scanner.ScanIgnoredFiles(tempDir, excluder)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}

	// 验证没有 .log 文件
	for _, file := range files {
		if filepath.Ext(file.AbsPath) == ".log" {
			t.Errorf("*.log 文件应该被排除: %s", file.AbsPath)
		}
	}
}

func TestScanIgnoredFilesWithProgressStreamConcurrent(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git 不在 PATH 中，跳过测试")
	}

	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")

	// 创建并初始化 Git 仓库
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	initGitRepo(t, repoDir)

	// 创建 .gitignore 和被忽略的文件
	createGitignore(t, repoDir, "*.log\ntemp/\n")
	createIgnoredFilesInRepo(t, repoDir)

	excluder, err := exclude.NewMatcher([]string{})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	// 使用并发扫描，设置 numWorkers=1 以确保确定性
	fileChan := make(chan scanner.IgnoredFileInfo, 100)
	errChan := make(chan error, 1)

	go func() {
		errChan <- scanner.ScanIgnoredFilesWithProgressStreamConcurrent(tempDir, excluder, nil, fileChan, 1)
		close(fileChan)
	}()

	var files []scanner.IgnoredFileInfo
	for file := range fileChan {
		files = append(files, file)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("并发扫描失败: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("期望找到被忽略的文件")
	}

	// 验证所有文件都来自正确的仓库
	for _, file := range files {
		if file.RepoRoot != repoDir {
			t.Errorf("文件应该来自仓库 %s，实际来自 %s", repoDir, file.RepoRoot)
		}

		// 验证绝对路径存在
		if _, err := os.Stat(file.AbsPath); os.IsNotExist(err) {
			t.Errorf("文件不存在: %s", file.AbsPath)
		}
	}
}

func TestScanIgnoredFilesWithProgressStreamConcurrent_MultipleWorkers(t *testing.T) {
	if !isGitAvailable() {
		t.Skip("Git 不在 PATH 中，跳过测试")
	}

	tempDir := t.TempDir()

	// 创建多个仓库来测试并发
	repoDirs := []string{"repo1", "repo2", "repo3"}
	for _, repoName := range repoDirs {
		repoDir := filepath.Join(tempDir, repoName)
		if err := os.MkdirAll(repoDir, 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}

		initGitRepo(t, repoDir)
		createGitignore(t, repoDir, "*.log\n")
		createIgnoredFile(t, repoDir, "test.log", "log content")
	}

	excluder, err := exclude.NewMatcher([]string{})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	// 使用多个 worker 进行并发扫描
	fileChan := make(chan scanner.IgnoredFileInfo, 100)
	errChan := make(chan error, 1)

	go func() {
		errChan <- scanner.ScanIgnoredFilesWithProgressStreamConcurrent(tempDir, excluder, nil, fileChan, 3)
		close(fileChan)
	}()

	var files []scanner.IgnoredFileInfo
	for file := range fileChan {
		files = append(files, file)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("并发扫描失败: %v", err)
	}

	// 应该找到 3 个文件（每个仓库一个 .log 文件）
	if len(files) != 3 {
		t.Errorf("期望找到 3 个文件，实际找到 %d 个", len(files))
	}

	// 验证所有仓库都被处理了
	repoFound := make(map[string]bool)
	for _, file := range files {
		repoFound[file.RepoRoot] = true
	}

	if len(repoFound) != 3 {
		t.Errorf("期望处理 3 个仓库，实际处理了 %d 个", len(repoFound))
	}
}

// createIgnoredFilesInRepo 创建测试用的被忽略文件
func createIgnoredFilesInRepo(t *testing.T, repo string) {
	files := map[string]string{
		"debug.log": "日志内容",
		"temp.tmp":  "临时文件",
		"data.txt":  "普通文件（不会被忽略）",
	}

	for relPath, content := range files {
		createIgnoredFile(t, repo, relPath, content)
	}
}

// createIgnoredFile 创建单个被忽略的文件
func createIgnoredFile(t *testing.T, repo, relPath, content string) {
	fullPath := filepath.Join(repo, relPath)

	// 创建目录
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("创建目录失败 %s: %v", dir, err)
	}

	// 创建文件
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("创建文件失败 %s: %v", fullPath, err)
	}
}

// TestFilterRedundantFiles 测试过滤冗余文件的逻辑
func TestFilterRedundantFiles(t *testing.T) {
	tempDir := t.TempDir()
	// 模拟searchRoot是tempDir的父目录，这样RelativePath的计算才是正确的
	repoName := filepath.Base(tempDir)

	// 创建测试文件结构
	testFiles := []string{
		"file1.txt",
		"dir1/file2.txt",
		"dir1/file3.txt",
		"dir1/subdir/file4.txt",
		"dir2/file5.txt",
	}

	var files []scanner.IgnoredFileInfo
	for _, relPath := range testFiles {
		fullPath := filepath.Join(tempDir, relPath)

		// 创建目录
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录失败 %s: %v", dir, err)
		}

		// 创建文件
		if err := os.WriteFile(fullPath, []byte("test"), 0644); err != nil {
			t.Fatalf("创建文件失败 %s: %v", fullPath, err)
		}

		// 计算相对于searchRoot的相对路径
		relToSearchRoot := filepath.Join(repoName, relPath)

		files = append(files, scanner.IgnoredFileInfo{
			AbsPath:      fullPath,
			RelativePath: relToSearchRoot,
			RepoRoot:     tempDir,
		})
	}

	// 测试过滤逻辑
	ignoredDirs := make(map[string]bool)
	filtered := scanner.FilterRedundantFiles(files, ignoredDirs)

	// 应该保留：repoName/file1.txt, repoName/dir2/file5.txt, repoName/dir1（因为dir1下有2个文件，被替换为目录）, repoName/dir1/subdir/file4.txt（因为subdir只有一个文件）
	expectedCount := 4
	if len(filtered) != expectedCount {
		t.Errorf("期望过滤后有 %d 个文件，实际有 %d 个", expectedCount, len(filtered))
		for i, f := range filtered {
			t.Logf("保留的文件 %d: %s", i, f.RelativePath)
		}
	}

	// 验证结果
	expectedDir1 := filepath.Join(repoName, "dir1")
	expectedFile1 := filepath.Join(repoName, "file1.txt")
	expectedDir2File5 := filepath.Join(repoName, "dir2/file5.txt")
	expectedSubdirFile := filepath.Join(repoName, "dir1/subdir/file4.txt")

	foundDir1 := false
	foundFile1 := false
	foundDir2File5 := false
	foundSubdirFile := false
	for _, f := range filtered {
		switch f.RelativePath {
		case expectedDir1:
			foundDir1 = true
		case expectedFile1:
			foundFile1 = true
		case expectedDir2File5:
			foundDir2File5 = true
		case expectedSubdirFile:
			foundSubdirFile = true
		}
	}
	if !foundDir1 {
		t.Error("期望dir1被替换为目录条目")
	}
	if !foundFile1 {
		t.Error("期望保留file1.txt")
	}
	if !foundDir2File5 {
		t.Error("期望保留dir2/file5.txt")
	}
	if !foundSubdirFile {
		t.Error("期望保留dir1/subdir/file4.txt")
	}
}
