package logics

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	cfgpkg "github.com/aogg/copy-ignore/src/config"
)

// sliceFlags 用于支持多个相同名称的标志
type sliceFlags []string

func (s *sliceFlags) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *sliceFlags) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// ParseFlags 解析命令行标志
func ParseFlags() *cfgpkg.Config {
	var excludes sliceFlags

	flag.Var(&excludes, "exclude", "排除模式（支持多次，可为绝对路径或通配符）")
	dryRun := flag.Bool("dry-run", false, "仅显示要复制的文件，不实际复制")
	concurrency := flag.Int("concurrency", 8, "并行复制的并发数")
	verbose := flag.Bool("verbose", false, "显示详细输出")
	flag.BoolVar(verbose, "v", false, "显示详细输出（简写）")
	backupKeep := flag.Int("backup-keep", 3, "每个备份目录保留的最近备份数")
	historySubDir := flag.String("history-subdir", "copy-ignore备份", "在备份目录下创建的子目录名称")
	historyDir := flag.String("history-dir", "", "备份历史文件夹")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s [选项] <搜索根目录> <备份根目录>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "将 Git 仓库中被忽略的文件复制到指定备份目录，保持目录结构。\n\n")
		fmt.Fprintf(os.Stderr, "参数:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s --exclude \"C:\\aaa\\qwe\\\" --exclude \"*\\vendor\" C:\\search D:\\backup\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --backup-keep 5 --backup-subdir \"old\" C:\\search D:\\backup\n", os.Args[0])
	}

	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		flag.Usage()
		os.Exit(1)
	}

	backupRoot := args[1]

	return &cfgpkg.Config{
		SearchRoot:   args[0],
		BackupRoot:   backupRoot,
		Excludes:     excludes,
		DryRun:       *dryRun,
		Concurrency:  *concurrency,
		Verbose:      *verbose,
		BackupDirs:   nil,
		BackupKeep:   *backupKeep,
		BackupSubdir: *historySubDir,
		HistoryDir:   *historyDir,
	}
}

// ValidateConfig 验证配置参数
func ValidateConfig(cfg *cfgpkg.Config) error {
	// 检查搜索根目录是否存在且为目录
	if info, err := os.Stat(cfg.SearchRoot); err != nil {
		return fmt.Errorf("搜索根目录不存在: %s", cfg.SearchRoot)
	} else if !info.IsDir() {
		return fmt.Errorf("搜索根目录不是目录: %s", cfg.SearchRoot)
	}

	// 检查备份根目录是否存在，不存在则创建
	if _, err := os.Stat(cfg.BackupRoot); os.IsNotExist(err) {
		if err := os.MkdirAll(cfg.BackupRoot, 0755); err != nil {
			return fmt.Errorf("创建备份根目录失败: %s (%v)", cfg.BackupRoot, err)
		}
	} else if info, err := os.Stat(cfg.BackupRoot); err != nil {
		return fmt.Errorf("访问备份根目录失败: %s (%v)", cfg.BackupRoot, err)
	} else if !info.IsDir() {
		return fmt.Errorf("备份根目录不是目录: %s", cfg.BackupRoot)
	}

	// 将 BackupRoot 添加到备份目录列表，用于备份功能
	cfg.BackupDirs = append(cfg.BackupDirs, cfg.BackupRoot)

	// 验证并发数
	if cfg.Concurrency <= 0 {
		return fmt.Errorf("并发数必须大于 0")
	}

	// 验证备份保留数
	if cfg.BackupKeep <= 0 {
		return fmt.Errorf("备份保留数必须大于 0")
	}

	// 归一化路径
	cfg.SearchRoot = filepath.Clean(cfg.SearchRoot)
	cfg.BackupRoot = filepath.Clean(cfg.BackupRoot)

	return nil
}
