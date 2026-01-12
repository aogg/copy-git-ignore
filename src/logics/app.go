package logics

import (
	"fmt"
	"log"
	"strings"
	"time"

	cfgpkg "github.com/aogg/copy-ignore/src/config"
	"github.com/aogg/copy-ignore/src/copy"
	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/aogg/copy-ignore/src/scanner"
)

// Run 运行主程序逻辑
func Run(excluder *exclude.Matcher) {
	cfg := cfgpkg.GetGlobalConfig()
	// 扫描所有 Git 仓库并获取被忽略的文件
	fmt.Printf("正在扫描目录: %s\n", cfg.SearchRoot)

	// 创建进度显示回调
	prevLen := 0
	maxLen := 100 // 限制最大显示长度，避免换行
	progress := func(path string) {
		// 截断过长的路径
		displayPath := path
		if len(displayPath) > maxLen {
			// 显示路径的开头部分和结尾部分
			displayPath = displayPath[:maxLen-3] + "..."
		}

		// 回到行首，打印新路径，如果新路径比旧路径短，用空格覆盖剩余部分
		fmt.Printf("\r当前扫描: %s", displayPath)
		if len(displayPath) < prevLen {
			fmt.Printf("%s", strings.Repeat(" ", prevLen-len(displayPath)))
		}
		prevLen = len(displayPath)
	}

	// 执行复制操作
	if cfg.DryRun {
		runDryRun(excluder, progress)
	} else {
		runCopy(excluder, progress)
	}
}

// runDryRun 执行干运行模式
func runDryRun(excluder *exclude.Matcher, progress func(string)) {
	cfg := cfgpkg.GetGlobalConfig()
	fmt.Println("干运行模式，不会实际复制文件")

	// 记录扫描开始时间
	scanStartTime := time.Now()
	fmt.Printf("扫描开始时间: %s\n", scanStartTime.Format("2006-01-02 15:04:05"))

	// 在dry-run模式下也需要扫描来显示文件，使用更大的缓冲区避免死锁
	fileChan := make(chan scanner.IgnoredFileInfo, 10000)
	var allFiles []scanner.IgnoredFileInfo

	// 启动收集协程
	collectDone := make(chan struct{})
	go func() {
		defer close(collectDone)
		for file := range fileChan {
			allFiles = append(allFiles, file)
		}
	}()

	// 扫描
	err := scanner.ScanIgnoredFilesWithProgressStream(cfg.SearchRoot, excluder, progress, fileChan)
	close(fileChan)
	<-collectDone

	// 记录扫描结束时间并计算耗时
	scanEndTime := time.Now()
	scanDuration := scanEndTime.Sub(scanStartTime)
	fmt.Printf("扫描结束时间: %s\n", scanEndTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("扫描耗时: %.2f秒\n", scanDuration.Seconds())

	if err != nil {
		log.Fatalf("扫描失败: %v", err)
	}

	// 显示找到的文件
	if cfg.Verbose && len(allFiles) > 0 {
		// 记录输出开始时间
		outputStartTime := time.Now()
		fmt.Printf("输出结果开始时间: %s\n", outputStartTime.Format("2006-01-02 15:04:05"))

		fmt.Printf("找到 %d 个需要处理的被忽略文件\n", len(allFiles))
		for _, file := range allFiles {
			fmt.Printf("  %s\n", file.RelativePath)
		}

		// 记录输出结束时间
		outputEndTime := time.Now()
		fmt.Printf("输出结果结束时间: %s\n", outputEndTime.Format("2006-01-02 15:04:05"))
	}
}

// runCopy 执行复制操作
func runCopy(excluder *exclude.Matcher, progress func(string)) {
	cfg := cfgpkg.GetGlobalConfig()
	fmt.Printf("正在复制到: %s\n", cfg.BackupRoot)

	// 创建文件channel，使用更大的缓冲区避免死锁
	fileChan := make(chan scanner.IgnoredFileInfo, 10000)

	// 用于控制输出频率，避免输出过于频繁
	lastOutputTime := time.Now()
	lastSrc := ""
	lastDest := ""

	// 进度回调函数
	onProgress := func(copied, skipped, errors, total int, src, dest string) {
		now := time.Now()
		// 每500ms最多输出一次，或者在路径发生变化时
		if now.Sub(lastOutputTime) > 500*time.Millisecond || src != lastSrc || dest != lastDest {
			fmt.Printf("\r进度: %d/%d 已复制, %d 跳过, %d 出错", copied, total, skipped, errors)
			lastOutputTime = now
			lastSrc = src
			lastDest = dest
		}
	}

	// 启动异步复制
	var copyResult *copy.CopyResult
	var copyErr error
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		copyResult, copyErr = copy.CopyFilesStreamWithProgress(
			fileChan,
			onProgress,
			excluder)
	}()

	// 流式扫描并发送文件到channel
	scanErr := scanner.ScanIgnoredFilesWithProgressStream(cfg.SearchRoot, excluder, progress, fileChan)
	close(fileChan) // 扫描完成，关闭channel

	if scanErr != nil {
		fmt.Println() // 换行以恢复正常输出
		log.Fatalf("扫描失败: %v", scanErr)
	}

	// 扫描完成，输出当前状态
	fmt.Println() // 换行以恢复正常输出
	fmt.Println("扫描完成，开始等待剩余复制任务...")

	// 等待复制完成
	<-copyDone

	if copyErr != nil {
		log.Fatalf("复制失败: %v", copyErr)
	}

	// 输出最终结果
	fmt.Printf("复制全部完成: %d 个文件处理，%d 个跳过", copyResult.Copied, copyResult.Skipped)
	if copyResult.Errors > 0 {
		fmt.Printf("，%d 个出错", copyResult.Errors)
	}
	fmt.Println()

	// 输出复制日志
	if len(copyResult.Logs) > 0 {
		for _, log := range copyResult.Logs {
			fmt.Println(log)
		}
	}
}
