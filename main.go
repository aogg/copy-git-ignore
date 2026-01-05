package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/aogg/copy-ignore/src/scanner"
	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/aogg/copy-ignore/src/copy"
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
	files, err := scanner.ScanIgnoredFiles(config.SearchRoot, excluder)
	if err != nil {
		log.Fatalf("扫描失败: %v", err)
	}

	if len(files) == 0 {
		fmt.Println("未找到需要备份的被忽略文件")
		return
	}

	if config.Verbose {
		fmt.Printf("找到 %d 个需要处理的被忽略文件\n", len(files))
		for _, file := range files {
			fmt.Printf("  %s\n", file)
		}
	}

	// 执行复制操作
	if config.DryRun {
		fmt.Println("干运行模式，不会实际复制文件")
	} else {
		fmt.Printf("正在复制到: %s\n", config.BackupRoot)

		result, err := copy.CopyFiles(files, config.BackupRoot, config.Concurrency, config.Verbose)
		if err != nil {
			log.Fatalf("复制失败: %v", err)
		}

		fmt.Printf("复制完成: %d 个文件处理，%d 个跳过\n", result.Copied, result.Skipped)
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
