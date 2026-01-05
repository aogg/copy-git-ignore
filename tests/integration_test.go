package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aogg/copy-ignore/src/copy"
	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/aogg/copy-ignore/src/scanner"
)

// TestIntegration 测试完整的复制流程
func TestIntegration(t *testing.T) {
	// 检查 git 是否可用
	if !isGitAvailable() {
		t.Skip("Git 不在 PATH 中，跳过集成测试")
	}

	// 创建临时目录
	tempDir := t.TempDir()
	searchRoot := filepath.Join(tempDir, "search")
	backupRoot := filepath.Join(tempDir, "backup")

	// 创建测试目录结构
	setupTestRepository(t, searchRoot)

	// 测试不带排除的情况
	t.Run("NoExcludes", func(t *testing.T) {
		testCopyWithoutExcludes(t, searchRoot, backupRoot)
	})

	// 测试带排除的情况
	t.Run("WithExcludes", func(t *testing.T) {
		testCopyWithExcludes(t, searchRoot, backupRoot)
	})

	// 测试增量复制
	t.Run("IncrementalCopy", func(t *testing.T) {
		testIncrementalCopy(t, searchRoot, backupRoot)
	})
}

// setupTestRepository 创建测试用的 Git 仓库结构
func setupTestRepository(t *testing.T, root string) {
	// 创建目录结构
	repo1 := filepath.Join(root, "repo1")
	repo2 := filepath.Join(root, "repo2")
	normalDir := filepath.Join(root, "normal")

	dirs := []string{repo1, repo2, normalDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("创建目录失败 %s: %v", dir, err)
		}
	}

	// 初始化 Git 仓库
	initGitRepo(t, repo1)
	initGitRepo(t, repo2)

	// 创建 .gitignore 文件
	createGitignore(t, repo1, "*.log\n*.tmp\nvendor/\n")
	createGitignore(t, repo2, "*.bak\nbuild/\n")

	// 创建被忽略的文件
	createIgnoredFiles(t, repo1)
	createIgnoredFiles(t, repo2)
}

// initGitRepo 初始化 Git 仓库
func initGitRepo(t *testing.T, dir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("初始化 Git 仓库失败 %s: %v\n输出: %s", dir, err, output)
	}

	// 注意：这里不做初始提交，因为 .gitignore 需要先提交，然后被忽略的文件才能正确工作
}

// createGitignore 创建 .gitignore 文件并提交
func createGitignore(t *testing.T, repo, content string) {
	gitignorePath := filepath.Join(repo, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("创建 .gitignore 失败: %v", err)
	}

	// 提交 .gitignore 文件
	cmd := exec.Command("git", "add", ".gitignore")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("添加 .gitignore 失败: %v", err)
	}

	cmd = exec.Command("git", "-c", "user.email=test@example.com", "-c", "user.name=Test User", "commit", "-m", "add gitignore")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("提交 .gitignore 失败: %v\n输出: %s", err, output)
	}
}

// createIgnoredFiles 创建被忽略的文件
func createIgnoredFiles(t *testing.T, repo string) {
	files := map[string]string{
		"debug.log":     "这是一些调试日志",
		"temp.tmp":      "临时文件内容",
		"vendor/lib.a":  "库文件",
		"app.bak":       "备份文件",
		"build/output":  "构建输出",
	}

	for relPath, content := range files {
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
}

// testCopyWithoutExcludes 测试不带排除的复制
func testCopyWithoutExcludes(t *testing.T, searchRoot, backupRoot string) {
	// 清理备份目录
	os.RemoveAll(backupRoot)

	excluder, err := exclude.NewMatcher([]string{})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	files, err := scanner.ScanIgnoredFiles(searchRoot, excluder)
	if err != nil {
		t.Fatalf("扫描文件失败: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("应该找到被忽略的文件")
	}

	// 验证找到的文件
	expectedFiles := map[string]bool{
		"repo1/debug.log":     true,
		"repo1/temp.tmp":      true,
		"repo1/vendor/lib.a":  true,
		"repo2/app.bak":       true,
		"repo2/build/output":  true,
	}

	foundFiles := make(map[string]bool)
	for _, file := range files {
		relPath := strings.TrimPrefix(file.AbsPath, searchRoot+string(os.PathSeparator))
		// 转换为正斜杠以便比较
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		foundFiles[relPath] = true
	}

	for expected := range expectedFiles {
		if !foundFiles[expected] {
			t.Errorf("缺少期望的文件: %s", expected)
		}
	}

	// 执行复制
	result, err := copy.CopyFiles(files, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}

	if result.Copied != len(files) {
		t.Errorf("期望复制 %d 个文件，实际复制 %d 个", len(files), result.Copied)
	}

	// 验证文件已被复制
	for _, file := range files {
		destPath := filepath.Join(backupRoot, file.RelativePath)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("文件未被复制: %s", destPath)
		}
	}
}

// testCopyWithExcludes 测试带排除的复制
func testCopyWithExcludes(t *testing.T, searchRoot, backupRoot string) {
	// 清理备份目录
	os.RemoveAll(backupRoot)

	// 使用排除模式：排除所有 *.log 文件和 vendor/ 目录
	excluder, err := exclude.NewMatcher([]string{"*.log", "*\\vendor\\*"})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	files, err := scanner.ScanIgnoredFiles(searchRoot, excluder)
	if err != nil {
		t.Fatalf("扫描文件失败: %v", err)
	}

	// 验证排除工作正常
	for _, file := range files {
		if strings.HasSuffix(file.AbsPath, ".log") {
			t.Errorf("*.log 文件应该被排除: %s", file.AbsPath)
		}
		if strings.Contains(file.AbsPath, "vendor") {
			t.Errorf("vendor 目录中的文件应该被排除: %s", file.AbsPath)
		}
	}

	// 执行复制
	result, err := copy.CopyFiles(files, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}

	if result.Copied != len(files) {
		t.Errorf("期望复制 %d 个文件，实际复制 %d 个", len(files), result.Copied)
	}
}

// testIncrementalCopy 测试增量复制
func testIncrementalCopy(t *testing.T, searchRoot, backupRoot string) {
	excluder, err := exclude.NewMatcher([]string{})
	if err != nil {
		t.Fatalf("创建排除匹配器失败: %v", err)
	}

	files, err := scanner.ScanIgnoredFiles(searchRoot, excluder)
	if err != nil {
		t.Fatalf("扫描文件失败: %v", err)
	}

	// 第一次复制
	_, err = copy.CopyFiles(files, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("第一次复制失败: %v", err)
	}

	// 再次复制（应该是增量复制，所有文件应该被跳过）
	result2, err := copy.CopyFiles(files, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("第二次复制失败: %v", err)
	}

	if result2.Copied != 0 {
		t.Errorf("增量复制时期望复制 0 个文件，实际复制 %d 个", result2.Copied)
	}

	if result2.Skipped != len(files) {
		t.Errorf("增量复制时期望跳过 %d 个文件，实际跳过 %d 个", len(files), result2.Skipped)
	}

	// 修改一个源文件的时间戳，使其看起来更新
	if len(files) > 0 {
		srcPath := files[0].AbsPath
		now := time.Now()
		oldTime := now.Add(-time.Hour) // 设为1小时前

		if err := os.Chtimes(srcPath, now, oldTime); err != nil {
			t.Logf("修改文件时间失败: %v", err)
		}

		// 再次复制，应该只复制那个文件
		result3, err := copy.CopyFiles([]scanner.IgnoredFileInfo{files[0]}, backupRoot, 2, false)
		if err != nil {
			t.Fatalf("第三次复制失败: %v", err)
		}

		if result3.Copied > 1 {
			t.Errorf("期望最多复制 1 个文件，实际复制 %d 个", result3.Copied)
		}
	}
}

// isGitAvailable 检查 git 是否可用
func isGitAvailable() bool {
	cmd := exec.Command("git", "--version")
	return cmd.Run() == nil
}
