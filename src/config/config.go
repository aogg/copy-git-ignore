package config

import "path/filepath"

// Config 包含程序的所有配置
type Config struct {
	SearchRoot   string   // 开始搜索的根目录
	BackupRoot   string   // 备份目标根目录
	Excludes     []string // 排除模式列表
	DryRun       bool     // 仅显示要复制的文件，不实际复制
	Concurrency  int      // 并行复制的并发数
	Verbose      bool     // 详细输出
	BackupDirs   []string // 备份目录列表（逗号分隔），默认会将 BackupRoot 添加到列表中
	BackupKeep   int      // 每个备份目录保留的备份数
	BackupSubdir string   // 在备份目录下创建的子目录名称
	HistoryDir   string   // 备份历史记录目录
	Timestamp    string   // 备份时间戳（在 main 入口处生成）
}

// 全局配置实例
var GlobalConfig *Config

// InitGlobalConfig 初始化全局配置
func InitGlobalConfig(c *Config) {
	GlobalConfig = c
}

// GetGlobalConfig 获取全局配置
func GetGlobalConfig() *Config {
	return GlobalConfig
}

func (c *Config) HandleHistoryDir(currentDir string) string {
	var baseDir string
	if c.HistoryDir != "" {
		baseDir = c.HistoryDir
	} else {
		baseDir = filepath.Join(currentDir, c.BackupSubdir)
	}
	return filepath.Join(baseDir, c.Timestamp)
}
