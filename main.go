package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aogg/copy-ignore/src/copy"
	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/aogg/copy-ignore/src/scanner"
)

// Config 包含程序的所有配置
type Config struct {
	SearchRoot  string   // 开始搜索的根目录
	BackupRoot  string   // 备份目标根目录
	Excludes    []string // 排除模式列表
	DryRun      bool     // 仅显示要复制的文件，不实际复制
	Concurrency int      // 并行复制的并发数
	Verbose     bool     // 详细输出
}

func main() {
	// 解析命令行参数
	config := parseFlags()

	// 验证参数
	if err := validateConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// 初始化排除匹配器
	excluder, err := exclude.NewMatcher(config.Excludes)
	if err != nil {
		log.Fatalf("初始化排除匹配器失败: %v", err)
	}

	// 扫描所有 Git 仓库并获取被忽略的文件
	fmt.Printf("正在扫描目录: %s\n", config.SearchRoot)

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
	if config.DryRun {
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
		err = scanner.ScanIgnoredFilesWithProgressStream(config.SearchRoot, excluder, progress, fileChan)
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
		if config.Verbose && len(allFiles) > 0 {
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

	} else {
		fmt.Printf("正在复制到: %s\n", config.BackupRoot)

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
				if src != "" && dest != "" {
					fmt.Printf("\r已复制: %s -> %s\n", src, dest)
				}
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
				config.BackupRoot,
				config.Concurrency,
				config.Verbose,
				onProgress)
		}()

		// 流式扫描并发送文件到channel
		scanErr := scanner.ScanIgnoredFilesWithProgressStream(config.SearchRoot, excluder, progress, fileChan)
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
	}
}

// parseFlags 解析命令行标志
func parseFlags() *Config {
	var excludes sliceFlags

	flag.Var(&excludes, "exclude", "排除模式（支持多次，可为绝对路径或通配符）")
	dryRun := flag.Bool("dry-run", false, "仅显示要复制的文件，不实际复制")
	concurrency := flag.Int("concurrency", 8, "并行复制的并发数")
	verbose := flag.Bool("verbose", false, "显示详细输出")
	flag.BoolVar(verbose, "v", false, "显示详细输出（简写）")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s [选项] <搜索根目录> <备份根目录>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "将 Git 仓库中被忽略的文件复制到指定备份目录，保持目录结构。\n\n")
		fmt.Fprintf(os.Stderr, "参数:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s --exclude \"C:\\aaa\\qwe\\\" --exclude \"*\\vendor\" C:\\search D:\\backup\n", os.Args[0])
	}

	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		flag.Usage()
		os.Exit(1)
	}

	return &Config{
		SearchRoot:  args[0],
		BackupRoot:  args[1],
		Excludes:    excludes,
		DryRun:      *dryRun,
		Concurrency: *concurrency,
		Verbose:     *verbose,
	}
}

// validateConfig 验证配置参数
func validateConfig(config *Config) error {
	// 检查搜索根目录是否存在且为目录
	if info, err := os.Stat(config.SearchRoot); err != nil {
		return fmt.Errorf("搜索根目录不存在: %s", config.SearchRoot)
	} else if !info.IsDir() {
		return fmt.Errorf("搜索根目录不是目录: %s", config.SearchRoot)
	}

	// 检查备份根目录是否存在，不存在则创建
	if _, err := os.Stat(config.BackupRoot); os.IsNotExist(err) {
		if err := os.MkdirAll(config.BackupRoot, 0755); err != nil {
			return fmt.Errorf("创建备份根目录失败: %s (%v)", config.BackupRoot, err)
		}
	} else if info, err := os.Stat(config.BackupRoot); err != nil {
		return fmt.Errorf("访问备份根目录失败: %s (%v)", config.BackupRoot, err)
	} else if !info.IsDir() {
		return fmt.Errorf("备份根目录不是目录: %s", config.BackupRoot)
	}

	// 验证并发数
	if config.Concurrency <= 0 {
		return fmt.Errorf("并发数必须大于 0")
	}

	// 归一化路径
	config.SearchRoot = filepath.Clean(config.SearchRoot)
	config.BackupRoot = filepath.Clean(config.BackupRoot)

	return nil
}

// sliceFlags 用于支持多个相同名称的标志
type sliceFlags []string

func (s *sliceFlags) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *sliceFlags) Set(value string) error {
	*s = append(*s, value)
	return nil
}
