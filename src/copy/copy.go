package copy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aogg/copy-ignore/src/scanner"
)

// CopyResult 复制操作的结果统计
type CopyResult struct {
	Copied int // 实际复制的文件数
	Skipped int // 跳过的文件数（目标文件较新或相同）
	Errors  int // 复制出错的文件数
}

// CopyFiles 并行复制文件列表到指定目录
func CopyFiles(files []scanner.IgnoredFileInfo, destRoot string, concurrency int, verbose bool) (*CopyResult, error) {
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
			copyWorker(jobs, results)
		}()
	}

	// 发送复制任务
	for _, file := range files {
		destPath := filepath.Join(destRoot, file.RelativePath)
		jobs <- copyJob{
			srcPath: file.AbsPath,
			destPath: destPath,
			verbose: verbose,
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

// copyJob 表示单个复制任务
type copyJob struct {
	srcPath  string
	destPath string
	verbose  bool
}

// copyResult 表示复制任务的结果
type copyResult struct {
	srcPath string
	skipped bool
	err     error
}

// copyWorker 执行复制工作的协程
func copyWorker(jobs <-chan copyJob, results chan<- copyResult) {
	for job := range jobs {
		skipped, err := copyFile(job.srcPath, job.destPath, job.verbose)
		results <- copyResult{
			srcPath: job.srcPath,
			skipped: skipped,
			err:     err,
		}
	}
}

// copyFile 复制单个文件，如果目标文件存在且较新则跳过
func copyFile(srcPath, destPath string, verbose bool) (skipped bool, err error) {
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
			if verbose {
				fmt.Printf("跳过 (目标较新): %s\n", srcPath)
			}
			return true, nil
		}
	} else if !os.IsNotExist(err) {
		// 其他错误
		return false, fmt.Errorf("检查目标文件失败: %v", err)
	}

	// 如果是目录，递归复制整个目录
	if srcInfo.IsDir() {
		return copyDir(srcPath, destPath, verbose)
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
		fmt.Printf("已复制: %s -> %s\n", srcPath, destPath)
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
func copyDir(srcPath, destPath string, verbose bool) (skipped bool, err error) {
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

		if entry.IsDir() {
			// 递归复制子目录
			if _, err := copyDir(srcEntryPath, destEntryPath, verbose); err != nil {
				return false, fmt.Errorf("复制子目录失败 %s: %v", srcEntryPath, err)
			}
		} else {
			// 复制文件
			if _, err := copyFile(srcEntryPath, destEntryPath, verbose); err != nil {
				return false, fmt.Errorf("复制文件失败 %s: %v", srcEntryPath, err)
			}
		}
	}

	if verbose {
		fmt.Printf("已复制目录: %s -> %s\n", srcPath, destPath)
	}

	return false, nil
}
