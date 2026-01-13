package helpers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aogg/copy-ignore/src/config"
)

// CleanupDeletedSrcFiles 清理已删除的源文件对应的目标文件
// targetPaths: 当前扫描到的目标文件路径集合 (destPath -> srcPath)
func CleanupDeletedSrcFiles(targetPaths map[string]string) {

	if config.GetGlobalConfig().Verbose {
		fmt.Printf("开始CleanupDeletedSrcFiles: %s\n", len(targetPaths))
	}

	cfg := config.GetGlobalConfig()
	// 遍历目标根目录
	pathHandleHistoryDir := cfg.HandleHistoryDir(cfg.BackupRoot)

	err := filepath.Walk(cfg.BackupRoot, func(destPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录，只处理文件
		if info.IsDir() {
			// 检查是否是备份子目录，如果是则跳过整个目录
			// 排除历史记录目录及其子目录
			if pathHandleHistoryDir != "" {
				// 检查 destPath 是否是 pathHandleHistoryDir 的子孙地址或相等
				if destPath == pathHandleHistoryDir || strings.HasPrefix(destPath, pathHandleHistoryDir+string(filepath.Separator)) {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// 检查目标文件是否在当前扫描的文件中
		_, exists := targetPaths[destPath]
		if exists {
			// 文件存在，不需要清理
			return nil
		}

		// 检查文件的父目录是否在 targetPaths 中
		// 如果父目录被作为整体复制（如 .vscode 目录），则不应删除其中的文件
		destDir := filepath.Dir(destPath)
		_, dirExists := targetPaths[destDir]
		if dirExists {
			// 父目录被复制，说明这是目录的一部分，不需要清理
			return nil
		}

		// 目标文件不在当前扫描中，说明源文件已被删除
		// 需要备份并删除目标文件
		if cfg.Verbose {
			fmt.Printf("检测到源文件已删除，准备备份目标文件: %s\n", destPath)
		}

		// 计算相对路径
		relPath, err := filepath.Rel(cfg.BackupRoot, destPath)
		if err != nil {
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "计算相对路径失败 %s: %v\n", destPath, err)
			}
			return nil
		}

		// 使用全局配置中的时间戳
		timestamp := cfg.Timestamp

		// 备份并删除目标文件
		for _, backupDir := range cfg.BackupDirs {
			if backupDir == "" {
				continue
			}

			// 如果指定了备份子目录，则添加到路径中
			backupBase := cfg.HandleHistoryDir(backupDir)

			if cfg.Verbose {
				fmt.Printf("备份目标文件: %s -> %s\n", destPath, backupBase)
			}
			if err := moveToBackup(destPath, backupBase, relPath, timestamp); err != nil {
				fmt.Fprintf(os.Stderr, "备份失败 %s: %v\n", destPath, err)
				continue
			}

			// 清理旧备份
			if err := pruneBackups(backupBase, relPath, cfg.BackupKeep, cfg.Verbose); err != nil {
				fmt.Fprintf(os.Stderr, "清理备份目录失败 %s: %v\n", backupBase, err)
				if cfg.Verbose {
				}
			}

			// 备份成功后删除目标文件
			if cfg.Verbose {
				fmt.Printf("源文件已删除，备份并移除目标文件: %s\n", destPath)
			}
			// 只需要在一个备份目录中处理即可，因为目标文件只有一个
			break
		}

		return nil
	})

	if err != nil {
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "遍历目标目录失败: %v\n", err)
		}
	}
}

// BackupPathIfModified 检查目标路径是否被修改，如果被修改则备份到指定的备份目录列表
// srcPath: 源路径
// destPath: 目标路径
func BackupPathIfModified(srcPath, destPath string) error {
	cfg := config.GetGlobalConfig()

	//if cfg.Verbose {
	//	fmt.Printf("开始BackupPathIfModified: %s -> %s\n", srcPath, destPath)
	//}

	// 检查源文件是否存在
	_, err := os.Stat(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 源文件不存在，删除目标文件
			return nil // 在后面统一处理删除
		}
		return fmt.Errorf("检查源文件失败: %v", err)
	}

	// 检查目标是否存在且被修改
	modified, err := isTargetModified(srcPath, destPath)
	if err != nil {
		return fmt.Errorf("检查目标是否被修改失败: %v", err)
	}

	if !modified {
		// 目标未被修改，删除目标文件（如果源已删除）
		_, err := os.Stat(srcPath)
		if os.IsNotExist(err) {
			// 源文件不存在，删除目标文件
			if err := removeDestIfExists(destPath, false); err != nil {
				return fmt.Errorf("删除目标文件失败: %v", err)
			}
		}
		return nil
	}

	// 对每个备份目录执行备份
	for _, backupDir := range cfg.BackupDirs {
		if backupDir == "" {
			continue // 跳过空目录
		}

		// 如果指定了备份子目录，则添加到路径中
		backupBase := cfg.HandleHistoryDir(backupDir)

		if err := copyRecursive(srcPath, backupBase); err != nil {
			return fmt.Errorf("备份到目录 %s 失败: %v", backupDir, err)
		}

	}

	return nil
}

// removeDestIfExists 如果目标文件存在则删除它
func removeDestIfExists(destPath string, verbose bool) error {
	if _, err := os.Stat(destPath); err != nil {
		if os.IsNotExist(err) {
			return nil // 目标不存在，无需删除
		}
		return fmt.Errorf("检查目标文件失败: %v", err)
	}
	// 目标存在，删除它
	if verbose {
		fmt.Printf("源文件已删除，移除目标文件: %s\n", destPath)
	}
	if err := os.RemoveAll(destPath); err != nil {
		return fmt.Errorf("删除目标文件失败: %v", err)
	}
	return nil
}

// isTargetModified 检查目标是否相对于源被修改（基于mtime）
func isTargetModified(srcPath, destPath string) (bool, error) {
	destInfo, err := os.Stat(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // 目标不存在，不算被修改
		}
		return false, err
	}

	if !destInfo.IsDir() {
		// 对于文件，直接比较mtime
		srcInfo, err := os.Stat(srcPath)
		if err != nil {
			return false, err
		}
		return destInfo.ModTime().After(srcInfo.ModTime()), nil
	}

	// 对于目录，递归检查是否有任何文件/子目录的mtime晚于源
	return isDirModified(srcPath, destPath)
}

// isDirModified 递归检查目录是否被修改
func isDirModified(srcPath, destPath string) (bool, error) {
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false, err
	}

	destInfo, err := os.Stat(destPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// 如果目标目录的修改时间晚于源目录，则认为被修改
	if destInfo.ModTime().After(srcInfo.ModTime()) {
		return true, nil
	}

	// 递归检查子项
	entries, err := os.ReadDir(destPath)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		srcEntryPath := filepath.Join(srcPath, entry.Name())
		destEntryPath := filepath.Join(destPath, entry.Name())

		if entry.IsDir() {
			modified, err := isDirModified(srcEntryPath, destEntryPath)
			if err != nil {
				return false, err
			}
			if modified {
				return true, nil
			}
		} else {
			// 检查文件是否存在且mtime晚于源
			destEntryInfo, err := os.Stat(destEntryPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return false, err
				}
				continue // 文件不存在，跳过
			}

			srcEntryInfo, err := os.Stat(srcEntryPath)
			if err != nil {
				return false, err
			}

			if destEntryInfo.ModTime().After(srcEntryInfo.ModTime()) {
				return true, nil
			}
		}
	}

	return false, nil
}

// getRelativePath 获取目标路径相对于源路径的相对路径
func getRelativePath(srcPath, destPath string) (string, error) {
	// 为了在备份目录中保持目标的原始位置，我们使用目标的绝对路径去掉卷标后作为相对路径。
	// 例如: D:\a\b\c -> a\b\c
	destAbs, err := filepath.Abs(destPath)
	if err != nil {
		return "", err
	}
	vol := filepath.VolumeName(destAbs) // Windows 下如 "D:"
	rel := strings.TrimPrefix(destAbs, vol)
	// 去掉可能的路径前导分隔符
	rel = strings.TrimLeft(rel, string(os.PathSeparator))
	return rel, nil
}

// moveToBackup 将目标路径移动到备份目录的时间戳子目录下
func moveToBackup(src string, destBase string, relPath string, timestamp string) error {
	// 构造备份目标路径：destBase/timestamp/relPath/
	backupTarget := filepath.Join(destBase, timestamp, relPath)

	// 确保备份目录存在
	if err := ensureDir(filepath.Dir(backupTarget)); err != nil {
		return fmt.Errorf("创建备份目录失败: %v", err)
	}

	// 尝试使用 os.Rename 进行快速移动（同设备）
	if config.GetGlobalConfig().Verbose {
		fmt.Printf("移动--moveToBackup: %s -> %s\n", src, backupTarget)
	}

	if err := os.Rename(src, backupTarget); err == nil {
		return nil // 成功移动
	}

	// Rename失败（可能是跨设备），回退到复制+删除
	if err := copyRecursive(src, backupTarget); err != nil {
		return fmt.Errorf("复制到备份目录失败: %v", err)
	}

	if config.GetGlobalConfig().Verbose {
		fmt.Printf("删除: %s\n", src)
	}

	// 删除原目录/文件
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("删除原路径失败: %v", err)
	}

	return nil
}

// ensureDir 确保目录存在，如果不存在则创建
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// copyRecursive 递归复制文件或目录
func copyRecursive(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDirRecursive(src, dest)
	}
	return copyFileContent(src, dest)
}

// copyDirRecursive 递归复制目录
func copyDirRecursive(src, dest string) error {
	if err := ensureDir(dest); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFileContent(srcPath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFileContent 复制文件内容
func copyFileContent(src, dest string) error {
	if config.GetGlobalConfig().Verbose {
		fmt.Fprintf(os.Stdout, "history: 复制文件 %s -> %s\n", src, dest)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// pruneBackups 清理备份，只保留最近的keep个备份
func pruneBackups(destBase, relPath string, keep int, verbose bool) error {
	backupDir := filepath.Join(destBase, relPath)

	// 获取所有时间戳目录
	timestamps, err := listTimestampedDirs(backupDir)
	if err != nil {
		return err
	}

	// 如果备份数不超过keep，直接返回
	if len(timestamps) <= keep {
		if verbose {
			fmt.Printf("备份目录 %s 当前备份数 %d，无需清理（保留 %d）\n", backupDir, len(timestamps), keep)
		}
		return nil
	}

	// 按时间戳排序（最新的在前）
	sort.Sort(sort.Reverse(sort.StringSlice(timestamps)))

	// 删除超出keep的旧备份
	for i := keep; i < len(timestamps); i++ {
		oldBackup := filepath.Join(backupDir, timestamps[i])
		if verbose {
			fmt.Printf("删除旧备份: %s\n", oldBackup)
		}
		if err := os.RemoveAll(oldBackup); err != nil {
			return fmt.Errorf("删除旧备份失败 %s: %v", oldBackup, err)
		}
	}

	return nil
}

// listTimestampedDirs 列出指定目录下的所有时间戳目录
func listTimestampedDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil // 目录不存在，返回空列表
		}
		return nil, err
	}

	var timestamps []string
	for _, entry := range entries {
		if entry.IsDir() {
			// 检查目录名是否符合时间戳格式 (YYYYMMDD-HHMMSS)
			name := entry.Name()
			if len(name) == 15 && strings.Contains(name, "-") {
				timestamps = append(timestamps, name)
			}
		}
	}

	return timestamps, nil
}
