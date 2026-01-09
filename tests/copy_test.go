package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aogg/copy-ignore/src/copy"
	"github.com/aogg/copy-ignore/src/scanner"
)

func TestCopyFiles_EmptyList(t *testing.T) {
	tempDir := t.TempDir()
	backupRoot := filepath.Join(tempDir, "backup")

	result, err := copy.CopyFiles([]scanner.IgnoredFileInfo{}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("复制空列表失败: %v", err)
	}

	if result.Copied != 0 || result.Skipped != 0 || result.Errors != 0 {
		t.Errorf("空列表复制结果不正确: %+v", result)
	}
}

func TestCopyFiles_SingleFile(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	backupRoot := filepath.Join(tempDir, "backup")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}

	srcFile := filepath.Join(srcDir, "test.txt")
	content := "测试文件内容"
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	// 创建文件信息
	fileInfo := scanner.IgnoredFileInfo{
		AbsPath:      srcFile,
		RelativePath: "test.txt",
		RepoRoot:     srcDir,
	}

	// 执行复制
	result, err := copy.CopyFiles([]scanner.IgnoredFileInfo{fileInfo}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}

	if result.Copied != 1 {
		t.Errorf("期望复制 1 个文件，实际复制 %d 个", result.Copied)
	}

	// 验证目标文件
	destFile := filepath.Join(backupRoot, "test.txt")
	destContent, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}

	if string(destContent) != content {
		t.Errorf("目标文件内容不匹配")
	}

	// 验证时间戳
	srcStat, err := os.Stat(srcFile)
	if err != nil {
		t.Fatalf("获取源文件状态失败: %v", err)
	}

	destStat, err := os.Stat(destFile)
	if err != nil {
		t.Fatalf("获取目标文件状态失败: %v", err)
	}

	if !srcStat.ModTime().Equal(destStat.ModTime()) {
		t.Errorf("目标文件时间戳不匹配: 源=%v, 目标=%v",
			srcStat.ModTime(), destStat.ModTime())
	}
}

func TestCopyFiles_IncrementalCopy_SkipNewer(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	backupRoot := filepath.Join(tempDir, "backup")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}

	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("原内容"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	fileInfo := scanner.IgnoredFileInfo{
		AbsPath:      srcFile,
		RelativePath: "test.txt",
		RepoRoot:     srcDir,
	}

	// 第一次复制
	_, err := copy.CopyFiles([]scanner.IgnoredFileInfo{fileInfo}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("第一次复制失败: %v", err)
	}

	destFile := filepath.Join(backupRoot, "test.txt")

	// 修改目标文件的时间戳，使其看起来比源文件新
	now := time.Now()
	newerTime := now.Add(time.Hour) // 1小时后

	if err := os.Chtimes(destFile, now, newerTime); err != nil {
		t.Fatalf("修改目标文件时间失败: %v", err)
	}

	// 第二次复制（增量复制）
	result2, err := copy.CopyFiles([]scanner.IgnoredFileInfo{fileInfo}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("第二次复制失败: %v", err)
	}

	if result2.Skipped != 1 {
		t.Errorf("增量复制期望跳过 1 个文件，实际跳过 %d 个", result2.Skipped)
	}

	if result2.Copied != 0 {
		t.Errorf("增量复制期望复制 0 个文件，实际复制 %d 个", result2.Copied)
	}
}

func TestCopyFiles_IncrementalCopy_UpdateOlder(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	backupRoot := filepath.Join(tempDir, "backup")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}

	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("原内容"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	fileInfo := scanner.IgnoredFileInfo{
		AbsPath:      srcFile,
		RelativePath: "test.txt",
		RepoRoot:     srcDir,
	}

	// 第一次复制
	_, err := copy.CopyFiles([]scanner.IgnoredFileInfo{fileInfo}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("第一次复制失败: %v", err)
	}

	destFile := filepath.Join(backupRoot, "test.txt")

	// 修改源文件，使其看起来更新
	now := time.Now()
	olderTime := now.Add(-time.Hour) // 1小时前

	if err := os.Chtimes(srcFile, now, olderTime); err != nil {
		t.Fatalf("修改源文件时间失败: %v", err)
	}

	// 等待一下确保时间差异
	time.Sleep(10 * time.Millisecond)

	// 更新源文件内容
	newContent := "新内容"
	if err := os.WriteFile(srcFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("更新源文件失败: %v", err)
	}

	// 第二次复制
	result2, err := copy.CopyFiles([]scanner.IgnoredFileInfo{fileInfo}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("第二次复制失败: %v", err)
	}

	if result2.Copied != 1 {
		t.Errorf("期望复制 1 个文件，实际复制 %d 个", result2.Copied)
	}

	// 验证内容已更新
	destContent, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}

	if string(destContent) != newContent {
		t.Errorf("目标文件内容未更新")
	}
}

func TestCopyFiles_NestedDirectories(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	backupRoot := filepath.Join(tempDir, "backup")

	// 创建嵌套目录结构
	nestedPath := filepath.Join(srcDir, "a", "b", "c")
	if err := os.MkdirAll(nestedPath, 0755); err != nil {
		t.Fatalf("创建嵌套目录失败: %v", err)
	}

	srcFile := filepath.Join(nestedPath, "deep.txt")
	if err := os.WriteFile(srcFile, []byte("深度文件"), 0644); err != nil {
		t.Fatalf("创建深度文件失败: %v", err)
	}

	fileInfo := scanner.IgnoredFileInfo{
		AbsPath:      srcFile,
		RelativePath: filepath.Join("a", "b", "c", "deep.txt"),
		RepoRoot:     srcDir,
	}

	// 执行复制
	result, err := copy.CopyFiles([]scanner.IgnoredFileInfo{fileInfo}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}

	if result.Copied != 1 {
		t.Errorf("期望复制 1 个文件，实际复制 %d 个", result.Copied)
	}

	// 验证目标文件和目录结构
	destFile := filepath.Join(backupRoot, "a", "b", "c", "deep.txt")
	if _, err := os.Stat(destFile); os.IsNotExist(err) {
		t.Errorf("目标文件不存在: %s", destFile)
	}

	// 验证目录被创建
	destDir := filepath.Dir(destFile)
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		t.Errorf("目标目录不存在: %s", destDir)
	}
}

func TestCopyFiles_SourceNotExist(t *testing.T) {
	tempDir := t.TempDir()
	backupRoot := filepath.Join(tempDir, "backup")

	// 创建不存在的源文件信息
	fileInfo := scanner.IgnoredFileInfo{
		AbsPath:      filepath.Join(tempDir, "nonexistent.txt"),
		RelativePath: "nonexistent.txt",
		RepoRoot:     tempDir,
	}

	result, err := copy.CopyFiles([]scanner.IgnoredFileInfo{fileInfo}, backupRoot, 2, false)
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}

	if result.Errors != 1 {
		t.Errorf("期望 1 个错误，实际 %d 个", result.Errors)
	}

	if result.Copied != 0 {
		t.Errorf("期望复制 0 个文件，实际复制 %d 个", result.Copied)
	}
}

func TestCopyFilesStreamWithProgress_EmptyChannel(t *testing.T) {
	tempDir := t.TempDir()
	backupRoot := filepath.Join(tempDir, "backup")

	fileChan := make(chan scanner.IgnoredFileInfo, 1)
	close(fileChan) // 空channel

	progressCalled := false
	onProgress := func(copied, skipped, errors, total int, lastSrc, lastDest string) {
		progressCalled = true
	}

	result, err := copy.CopyFilesStreamWithProgress(fileChan, backupRoot, 2, false, []string{}, 3, "", onProgress)
	if err != nil {
		t.Fatalf("流式复制空channel失败: %v", err)
	}

	if result.Copied != 0 || result.Skipped != 0 || result.Errors != 0 {
		t.Errorf("空channel复制结果不正确: %+v", result)
	}

	if progressCalled {
		t.Errorf("空channel不应该调用进度回调")
	}
}

func TestCopyFilesStreamWithProgress_SingleFile(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	backupRoot := filepath.Join(tempDir, "backup")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}

	srcFile := filepath.Join(srcDir, "test.txt")
	content := "测试文件内容"
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	fileChan := make(chan scanner.IgnoredFileInfo, 1)

	// 创建文件信息
	fileInfo := scanner.IgnoredFileInfo{
		AbsPath:      srcFile,
		RelativePath: "test.txt",
		RepoRoot:     srcDir,
	}
	fileChan <- fileInfo
	close(fileChan)

	var progressCalls []struct {
		copied, skipped, errors, total int
		src, dest                      string
	}

	onProgress := func(copied, skipped, errors, total int, lastSrc, lastDest string) {
		progressCalls = append(progressCalls, struct {
			copied, skipped, errors, total int
			src, dest                      string
		}{copied, skipped, errors, total, lastSrc, lastDest})
	}

	// 执行流式复制
	result, err := copy.CopyFilesStreamWithProgress(fileChan, backupRoot, 2, false, []string{}, 3, "", onProgress)
	if err != nil {
		t.Fatalf("流式复制失败: %v", err)
	}

	if result.Copied != 1 {
		t.Errorf("期望复制 1 个文件，实际复制 %d 个", result.Copied)
	}

	// 验证进度回调被调用
	if len(progressCalls) == 0 {
		t.Errorf("进度回调没有被调用")
	}

	// 验证最后一次回调包含正确的路径
	lastCall := progressCalls[len(progressCalls)-1]
	if lastCall.src != srcFile {
		t.Errorf("最后回调的源路径不正确: 期望 %s, 实际 %s", srcFile, lastCall.src)
	}

	expectedDest := filepath.Join(backupRoot, "test.txt")
	if lastCall.dest != expectedDest {
		t.Errorf("最后回调的目标路径不正确: 期望 %s, 实际 %s", expectedDest, lastCall.dest)
	}

	// 验证目标文件
	destFile := filepath.Join(backupRoot, "test.txt")
	destContent, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}

	if string(destContent) != content {
		t.Errorf("目标文件内容不匹配")
	}
}

func TestCopyFilesStreamWithProgress_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	backupRoot := filepath.Join(tempDir, "backup")

	fileChan := make(chan scanner.IgnoredFileInfo, 1)

	// 创建不存在的源文件信息
	fileInfo := scanner.IgnoredFileInfo{
		AbsPath:      filepath.Join(tempDir, "nonexistent.txt"),
		RelativePath: "nonexistent.txt",
		RepoRoot:     tempDir,
	}
	fileChan <- fileInfo
	close(fileChan)

	var progressCalls []struct {
		copied, skipped, errors, total int
		src, dest                      string
	}

	onProgress := func(copied, skipped, errors, total int, lastSrc, lastDest string) {
		progressCalls = append(progressCalls, struct {
			copied, skipped, errors, total int
			src, dest                      string
		}{copied, skipped, errors, total, lastSrc, lastDest})
	}

	// 执行流式复制
	result, err := copy.CopyFilesStreamWithProgress(fileChan, backupRoot, 2, false, []string{}, 3, "", onProgress)
	if err != nil {
		t.Fatalf("流式复制失败: %v", err)
	}

	if result.Errors != 1 {
		t.Errorf("期望 1 个错误，实际 %d 个", result.Errors)
	}

	if result.Copied != 0 {
		t.Errorf("期望复制 0 个文件，实际复制 %d 个", result.Copied)
	}

	// 验证进度回调包含错误信息
	if len(progressCalls) == 0 {
		t.Errorf("进度回调没有被调用")
	}

	lastCall := progressCalls[len(progressCalls)-1]
	if lastCall.errors != 1 {
		t.Errorf("最后回调的错误数不正确: 期望 1, 实际 %d", lastCall.errors)
	}
}
