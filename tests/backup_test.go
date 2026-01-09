package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aogg/copy-ignore/src/helpers"
)

// TestBackupPathIfModified_NoBackupNeeded 测试无需备份的情况
func TestBackupPathIfModified_NoBackupNeeded(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destDir := filepath.Join(tempDir, "dest")
	backupDir := filepath.Join(tempDir, "backup")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	// 目标不存在，应该无需备份
	err := helpers.BackupPathIfModified(srcFile, destDir, []string{backupDir}, 3, "")
	if err != nil {
		t.Errorf("目标不存在时备份失败: %v", err)
	}

	// 检查备份目录是否为空
	entries, err := os.ReadDir(backupDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("读取备份目录失败: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("目标不存在时不应该创建备份，实际创建了 %d 项", len(entries))
	}
}

// TestBackupPathIfModified_BackupNeeded 测试需要备份的情况
func TestBackupPathIfModified_BackupNeeded(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destDir := filepath.Join(tempDir, "dest")
	backupDir := filepath.Join(tempDir, "backup")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("source content"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	// 创建目标文件，修改时间晚于源文件
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("创建目标目录失败: %v", err)
	}
	destFile := filepath.Join(destDir, "test.txt")
	if err := os.WriteFile(destFile, []byte("dest content"), 0644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	// 修改目标文件的时间戳，使其看起来比源文件新
	now := time.Now()
	newerTime := now.Add(time.Hour)
	if err := os.Chtimes(destFile, now, newerTime); err != nil {
		t.Fatalf("修改目标文件时间失败: %v", err)
	}

	// 执行备份
	err := helpers.BackupPathIfModified(srcFile, destFile, []string{backupDir}, 3, "")
	if err != nil {
		t.Errorf("备份失败: %v", err)
	}

	// 检查备份是否创建
	backupEntries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("读取备份目录失败: %v", err)
	}

	if len(backupEntries) == 0 {
		t.Errorf("应该创建备份但没有")
	}

	// 检查备份内容
	for _, entry := range backupEntries {
		if entry.IsDir() && strings.Contains(entry.Name(), "-") {
			// 这是时间戳目录，检查内容
			backupPath := filepath.Join(backupDir, entry.Name(), "test.txt")
			content, err := os.ReadFile(backupPath)
			if err != nil {
				t.Errorf("读取备份文件失败: %v", err)
			} else if string(content) != "dest content" {
				t.Errorf("备份内容不正确: 期望 'dest content', 实际 '%s'", string(content))
			}
		}
	}
}

// TestBackupPathIfModified_MultipleBackupDirs 测试多个备份目录
func TestBackupPathIfModified_MultipleBackupDirs(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destDir := filepath.Join(tempDir, "dest")
	backupDir1 := filepath.Join(tempDir, "backup1")
	backupDir2 := filepath.Join(tempDir, "backup2")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("source"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	// 创建目标文件
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("创建目标目录失败: %v", err)
	}
	destFile := filepath.Join(destDir, "test.txt")
	if err := os.WriteFile(destFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	// 修改目标文件时间
	now := time.Now()
	newerTime := now.Add(time.Hour)
	if err := os.Chtimes(destFile, now, newerTime); err != nil {
		t.Fatalf("修改目标文件时间失败: %v", err)
	}

	// 执行备份到多个目录
	backupDirs := []string{backupDir1, backupDir2}
	err := helpers.BackupPathIfModified(srcFile, destFile, backupDirs, 3, "")
	if err != nil {
		t.Errorf("多目录备份失败: %v", err)
	}

	// 检查两个备份目录都创建了备份
	for _, backupDir := range backupDirs {
		entries, err := os.ReadDir(backupDir)
		if err != nil {
			t.Fatalf("读取备份目录 %s 失败: %v", backupDir, err)
		}
		if len(entries) == 0 {
			t.Errorf("备份目录 %s 应该有备份但为空", backupDir)
		}
	}
}

// TestBackupPathIfModified_BackupRotation 测试备份轮换
func TestBackupPathIfModified_BackupRotation(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destDir := filepath.Join(tempDir, "dest")
	backupDir := filepath.Join(tempDir, "backup")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("source"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	// 创建目标文件
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("创建目标目录失败: %v", err)
	}
	destFile := filepath.Join(destDir, "test.txt")

	// 创建5次备份，测试只保留3份
	for i := 0; i < 5; i++ {
		// 更新目标文件内容和时间
		content := "modified " + string(rune('0'+i))
		if err := os.WriteFile(destFile, []byte(content), 0644); err != nil {
			t.Fatalf("更新目标文件失败: %v", err)
		}

		now := time.Now()
		newerTime := now.Add(time.Hour)
		if err := os.Chtimes(destFile, now, newerTime); err != nil {
			t.Fatalf("修改目标文件时间失败: %v", err)
		}

		// 执行备份
		err := helpers.BackupPathIfModified(srcFile, destFile, []string{backupDir}, 3, "")
		if err != nil {
			t.Errorf("备份 %d 失败: %v", i, err)
		}

		// 等待一下确保时间戳不同
		time.Sleep(10 * time.Millisecond)
	}

	// 检查只保留了3个备份
	backupEntries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("读取备份目录失败: %v", err)
	}

	timestampDirs := 0
	for _, entry := range backupEntries {
		if entry.IsDir() && strings.Contains(entry.Name(), "-") {
			timestampDirs++
		}
	}

	if timestampDirs != 3 {
		t.Errorf("期望保留 3 个备份，实际保留 %d 个", timestampDirs)
	}
}

// TestBackupPathIfModified_DirectoryBackup 测试目录备份
func TestBackupPathIfModified_DirectoryBackup(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destDir := filepath.Join(tempDir, "dest")
	backupDir := filepath.Join(tempDir, "backup")

	// 创建源目录结构
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatalf("创建源子目录失败: %v", err)
	}
	srcFile1 := filepath.Join(srcDir, "file1.txt")
	srcFile2 := filepath.Join(srcDir, "subdir", "file2.txt")
	if err := os.WriteFile(srcFile1, []byte("content1"), 0644); err != nil {
		t.Fatalf("创建源文件1失败: %v", err)
	}
	if err := os.WriteFile(srcFile2, []byte("content2"), 0644); err != nil {
		t.Fatalf("创建源文件2失败: %v", err)
	}

	// 创建目标目录结构，并修改使其看起来更新
	if err := os.MkdirAll(filepath.Join(destDir, "subdir"), 0755); err != nil {
		t.Fatalf("创建目标子目录失败: %v", err)
	}
	destFile1 := filepath.Join(destDir, "file1.txt")
	destFile2 := filepath.Join(destDir, "subdir", "file2.txt")
	if err := os.WriteFile(destFile1, []byte("modified1"), 0644); err != nil {
		t.Fatalf("创建目标文件1失败: %v", err)
	}
	if err := os.WriteFile(destFile2, []byte("modified2"), 0644); err != nil {
		t.Fatalf("创建目标文件2失败: %v", err)
	}

	// 修改目标目录的时间戳
	now := time.Now()
	newerTime := now.Add(time.Hour)
	if err := os.Chtimes(destDir, now, newerTime); err != nil {
		t.Fatalf("修改目标目录时间失败: %v", err)
	}

	// 执行目录备份
	err := helpers.BackupPathIfModified(srcDir, destDir, []string{backupDir}, 3, "")
	if err != nil {
		t.Errorf("目录备份失败: %v", err)
	}

	// 检查备份结构
	backupEntries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("读取备份目录失败: %v", err)
	}

	if len(backupEntries) == 0 {
		t.Errorf("应该创建目录备份但没有")
	}

	// 查找时间戳目录
	var timestampDir string
	for _, entry := range backupEntries {
		if entry.IsDir() && strings.Contains(entry.Name(), "-") {
			timestampDir = entry.Name()
			break
		}
	}

	if timestampDir == "" {
		t.Errorf("没有找到时间戳备份目录")
		return
	}

	// 检查备份内容
	backupFile1 := filepath.Join(backupDir, timestampDir, "file1.txt")
	backupFile2 := filepath.Join(backupDir, timestampDir, "subdir", "file2.txt")

	content1, err := os.ReadFile(backupFile1)
	if err != nil {
		t.Errorf("读取备份文件1失败: %v", err)
	} else if string(content1) != "modified1" {
		t.Errorf("备份文件1内容不正确")
	}

	content2, err := os.ReadFile(backupFile2)
	if err != nil {
		t.Errorf("读取备份文件2失败: %v", err)
	} else if string(content2) != "modified2" {
		t.Errorf("备份文件2内容不正确")
	}
}

// TestBackupPathIfModified_EmptyBackupDirs 测试空备份目录列表
func TestBackupPathIfModified_EmptyBackupDirs(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destDir := filepath.Join(tempDir, "dest")

	// 创建源文件
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("创建源目录失败: %v", err)
	}
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0644); err != nil {
		t.Fatalf("创建源文件失败: %v", err)
	}

	// 创建目标文件
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatalf("创建目标目录失败: %v", err)
	}
	destFile := filepath.Join(destDir, "test.txt")
	if err := os.WriteFile(destFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("创建目标文件失败: %v", err)
	}

	// 修改目标文件时间
	now := time.Now()
	newerTime := now.Add(time.Hour)
	if err := os.Chtimes(destFile, now, newerTime); err != nil {
		t.Fatalf("修改目标文件时间失败: %v", err)
	}

	// 使用空备份目录列表
	err := helpers.BackupPathIfModified(srcFile, destFile, []string{}, 3, "")
	if err != nil {
		t.Errorf("空备份目录列表应该成功但失败: %v", err)
	}

	// 目标文件应该保持不变
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("读取目标文件失败: %v", err)
	}
	if string(content) != "modified" {
		t.Errorf("目标文件内容不应该改变")
	}
}
