package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aogg/copy-ignore/src/config"
	"github.com/aogg/copy-ignore/src/exclude"
	"github.com/aogg/copy-ignore/src/logics"
)

func main() {
	// 生成时间戳（入口处统一生成）
	timestamp := time.Now().Format("20060102-150405")

	// 解析命令行参数
	cfg := logics.ParseFlags()

	// 设置时间戳
	cfg.Timestamp = timestamp

	// 初始化全局配置
	config.InitGlobalConfig(cfg)

	// 验证参数
	if err := logics.ValidateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// 初始化排除匹配器
	excluder, err := exclude.NewMatcher(cfg.Excludes)
	if err != nil {
		log.Fatalf("初始化排除匹配器失败: %v", err)
	}

	// 运行主程序逻辑
	logics.Run(excluder)
}
