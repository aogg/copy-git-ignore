package copy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aogg/copy-ignore/src/config"
	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/aogg/copy-ignore/src/helpers"
	"github.com/aogg/copy-ignore/src/scanner"
)

// CopyResult 复制操作的结果统计
type CopyResult struct {
	Copied  int      // 实际复制的文件数
	Skipped int      // 跳过的文件数（目标文件较新或相同）
	Errors  int      // 复制出错的文件数
	Logs    []string // 复制日志（延迟输出）
}

// RealTimeCopyResult 支持实时统计的复制结果
type RealTimeCopyResult struct {
	mu      sync.RWMutex
	Copied  int // 实际复制的文件数
	Skipped int // 跳过的文件数
	Errors  int // 复制出错的文件数
	Total   int // 总文件数（实时更新）
}

// AddResult 线程安全地添加复制结果
func (r *RealTimeCopyResult) AddResult(copied, skipped, errors int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Copied += copied
	r.Skipped += skipped
	r.Errors += errors
}

// GetCurrentStats 获取当前统计（线程安全）
func (r *RealTimeCopyResult) GetCurrentStats() (copied, skipped, errors, total int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Copied, r.Skipped, r.Errors, r.Total
}

// SetTotal 设置总数
func (r *RealTimeCopyResult) SetTotal(total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Total = total
}

// CopyFiles 并行复制文件列表到指定目录
func CopyFiles(files []scanner.IgnoredFileInfo, destRoot string, concurrency int, verbose bool, excluder *exclude.Matcher) (*CopyResult, error) {
	if len(files) == 0 {
		return &CopyResult{}, nil
	}

	// 创建工作池
	jobs := make(chan copyJob, len(files))
	results := make(chan copyResult, len(files))

	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copyWorker(jobs, results, excluder)
		}()
	}

	// 发送复制任务
	for _, file := range files {
		destPath := filepath.Join(destRoot, file.RelativePath)
		jobs <- copyJob{
			srcPath:  file.AbsPath,
			destPath: destPath,
			verbose:  verbose,
		}
	}
	close(jobs)

	// 等待所有工作完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果
	result := &CopyResult{}
	for res := range results {
		if res.err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "复制失败 %s: %v\n", res.srcPath, res.err)
			}
			result.Errors++
		} else if res.skipped {
			result.Skipped++
		} else {
			result.Copied++
		}
	}

	return result, nil
}

// CopyFilesStreamWithProgress 从channel接收文件并异步复制，支持实时进度反馈
func CopyFilesStreamWithProgress(
	fileChan <-chan scanner.IgnoredFileInfo,
	onProgress func(copied, skipped, errors, total int, lastSrc, lastDest string), // 进度回调
	excluder *exclude.Matcher,
) (*CopyResult, error) {
	cfg := config.GetGlobalConfig()

	result := &RealTimeCopyResult{}
	var logMutex sync.Mutex
	var logs []string

	// 创建工作池，使用更大的缓冲区避免死锁
	jobs := make(chan copyJob, 1000)
	results := make(chan copyResult, 1000)

	// 启动工作协程
	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copyWorker(jobs, results, excluder)
		}()
	}

	// 启动结果收集器
	go func() {
		wg.Wait()
		close(results)
	}()

	// 从文件channel接收并发送到jobs，同时更新总数
	go func() {
		fileCount := 0
		targetPaths := make(map[string]string) // destPath -> srcPath，用于清理检查

		for file := range fileChan {
			destPath := filepath.Join(cfg.BackupRoot, file.RelativePath)
			jobs <- copyJob{
				srcPath:  file.AbsPath,
				destPath: destPath,
				verbose:  cfg.Verbose,
				logWriter: func(msg string) {
					logMutex.Lock()
					logs = append(logs, msg)
					logMutex.Unlock()
				},
			}
			fileCount++
			result.SetTotal(fileCount)
			targetPaths[destPath] = file.AbsPath
		}

		// 清理已删除的源文件对应的目标文件
		if len(cfg.BackupDirs) > 0 {
			helpers.CleanupDeletedSrcFiles(targetPaths)
		}

		close(jobs)
	}()

	// 收集结果并实时反馈
	for res := range results {
		if res.err != nil {
			result.AddResult(0, 0, 1)
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "复制失败 %s: %v\n", res.srcPath, res.err)
			}
		} else if res.skipped {
			result.AddResult(0, 1, 0)
		} else {
			result.AddResult(1, 0, 0)
		}

		// 实时调用进度回调
		if onProgress != nil {
			copied, skipped, errors, total := result.GetCurrentStats()
			onProgress(copied, skipped, errors, total, res.srcPath, res.destPath)
		}
	}

	// 返回最终结果
	finalCopied, finalSkipped, finalErrors, _ := result.GetCurrentStats()
	return &CopyResult{
		Copied:  finalCopied,
		Skipped: finalSkipped,
		Errors:  finalErrors,
		Logs:    logs,
	}, nil
}

// copyJob 表示单个复制任务
type copyJob struct {
	srcPath   string
	destPath  string
	verbose   bool
	logWriter func(string)
}

// copyResult 表示复制任务的结果
type copyResult struct {
	srcPath  string
	destPath string
	skipped  bool
	err      error
}

// copyWorker 执行复制工作的协程
func copyWorker(jobs <-chan copyJob, results chan<- copyResult, excluder *exclude.Matcher) {
	for job := range jobs {
		skipped, err := copyFile(job.srcPath, job.destPath, job.verbose, job.logWriter, excluder)
		results <- copyResult{
			srcPath:  job.srcPath,
			destPath: job.destPath,
			skipped:  skipped,
			err:      err,
		}
	}
}

// copyFile 复制单个文件，如果目标文件存在且较新则跳过
func copyFile(srcPath, destPath string, verbose bool, logWriter func(string), excluder *exclude.Matcher) (skipped bool, err error) {
	cfg := config.GetGlobalConfig()

	// 获取源文件信息
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false, fmt.Errorf("获取源文件信息失败: %v", err)
	}

	// 检查目标文件是否存在
	destInfo, err := os.Stat(destPath)
	if err == nil {
		// 目标文件存在，比较修改时间
		if srcInfo.ModTime().Before(destInfo.ModTime()) ||
			srcInfo.ModTime().Equal(destInfo.ModTime()) {
			// 源文件不比目标文件新，跳过复制
			//if verbose {
			//	logWriter(fmt.Sprintf("跳过 (目标较新): %s", srcPath))
			//}
			return true, nil
		}

		// 源文件比目标文件新，需要覆盖，先备份目标文件
		if len(cfg.BackupDirs) > 0 {
			if err := helpers.BackupFileBeforeOverwrite(destPath); err != nil {
				// 备份失败不应该阻止复制，只记录错误
				if verbose {
					fmt.Fprintf(os.Stderr, "备份失败 %s: %v\n", destPath, err)
				}
			}
		}
	} else if !os.IsNotExist(err) {
		// 其他错误
		return false, fmt.Errorf("检查目标文件失败: %v", err)
	}

	// 如果是目录，递归复制整个目录
	if srcInfo.IsDir() {
		return copyDir(srcPath, destPath, verbose, logWriter, excluder)
	}

	// 需要复制：创建目标目录
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return false, fmt.Errorf("创建目标目录失败: %v", err)
	}

	// 原子复制：先写入临时文件，再重命名
	tempPath := destPath + ".tmp"
	if err := copyFileContent(srcPath, tempPath); err != nil {
		// 清理临时文件
		os.Remove(tempPath)
		return false, fmt.Errorf("复制文件内容失败: %v", err)
	}

	// 原子重命名
	if err := os.Rename(tempPath, destPath); err != nil {
		// 清理临时文件
		os.Remove(tempPath)
		return false, fmt.Errorf("重命名文件失败: %v", err)
	}

	// 设置目标文件的修改时间为源文件的修改时间
	now := time.Now()
	if err := os.Chtimes(destPath, now, srcInfo.ModTime()); err != nil {
		// 这不是致命错误，只是记录警告
		if verbose {
			fmt.Fprintf(os.Stderr, "警告: 设置文件时间失败 %s: %v\n", destPath, err)
		}
	}

	if verbose {
		logWriter(fmt.Sprintf("已复制: %s -> %s", srcPath, destPath))
	}

	return false, nil
}

// copyFileContent 复制文件内容
func copyFileContent(srcPath, destPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	// 确保数据写入磁盘
	return destFile.Sync()
}

// copyDir 递归复制目录
func copyDir(srcPath, destPath string, verbose bool, logWriter func(string), excluder *exclude.Matcher) (skipped bool, err error) {
	// 创建目标目录
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return false, fmt.Errorf("创建目标目录失败: %v", err)
	}

	// 读取源目录内容
	entries, err := os.ReadDir(srcPath)
	if err != nil {
		return false, fmt.Errorf("读取源目录失败: %v", err)
	}

	// 递归复制所有文件和子目录
	for _, entry := range entries {
		srcEntryPath := filepath.Join(srcPath, entry.Name())
		destEntryPath := filepath.Join(destPath, entry.Name())

		// 检查是否应该排除此路径
		if excluder != nil && excluder.ShouldExclude(srcEntryPath) {
			if verbose {
				logWriter(fmt.Sprintf("跳过 (排除规则): %s", srcEntryPath))
			}
			continue
		}

		if entry.IsDir() {
			// 递归复制子目录
			if _, err := copyDir(srcEntryPath, destEntryPath, verbose, logWriter, excluder); err != nil {
				return false, fmt.Errorf("复制子目录失败 %s: %v", srcEntryPath, err)
			}
		} else {
			// 复制文件
			if _, err := copyFile(srcEntryPath, destEntryPath, verbose, logWriter, excluder); err != nil {
				return false, fmt.Errorf("复制文件失败 %s: %v", srcEntryPath, err)
			}
		}
	}

	//if verbose {
	//	logWriter(fmt.Sprintf("已复制目录: %s -> %s", srcPath, destPath))
	//}

	return false, nil
}
