# copy-ignore

一个 Windows Go 工具，用于将 Git 仓库中被忽略的文件复制到统一的备份目录，保持原始目录结构。

## 功能特点

- 递归扫描指定目录，查找所有 Git 仓库（包含 `.git` 目录的目录）
- 使用系统 Git CLI 列出被忽略的文件
- 流式处理：扫描到文件立即开始异步复制，提供实时进度反馈
- 支持增量复制：根据文件修改时间判断是否需要复制
- 支持多个排除模式（绝对路径或 glob 通配符）
- 并行复制，提高性能
- 保持原始目录结构
- 实时显示复制进度和路径映射

## 安装

确保系统中安装了 Go 和 Git，然后构建 Windows exe：

```bash
# 快速构建（使用构建脚本）
.\build.bat

# 或手动构建：
# 标准构建
go build -o copy-ignore.exe main.go

# 优化构建（减小文件大小）
go build -ldflags="-s -w" -o copy-ignore-release.exe main.go
```

## 使用方法

```bash
copy-ignore [选项] <搜索根目录> <备份根目录>
```

### 选项

- `--exclude <模式>`: 排除模式（可多次使用）
  - 绝对路径：`C:\path\to\exclude`
  - glob 模式：`*.log`、`**/vendor/**` 等
- `--dry-run`: 仅显示将要复制的文件，不实际复制
- `--concurrency <数字>`: 并行复制的并发数（默认 8）
- `--verbose, -v`: 显示详细输出

### 示例

```bash
# 基本用法
copy-ignore C:\projects D:\backup

# 排除特定目录和文件类型
copy-ignore --exclude "C:\projects\temp" --exclude "*.log" --exclude "**/vendor/**" C:\projects D:\backup

# 干运行模式
copy-ignore --dry-run --exclude "*.tmp" C:\projects D:\backup
```

### 输出示例

```
正在扫描目录: C:\projects
已复制: C:\projects\repo1\config\local.env -> D:\backup\repo1\config\local.env
进度: 45/120 已复制, 3 跳过, 0 出错
已复制: C:\projects\repo2\logs\debug.log -> D:\backup\repo2\logs\debug.log
进度: 67/120 已复制, 5 跳过, 1 出错
已复制: C:\projects\repo3\temp\cache.db -> D:\backup\repo3\temp\cache.db
进度: 89/120 已复制, 7 跳过, 1 出错
扫描完成，开始等待剩余复制任务...
进度: 105/120 已复制, 8 跳过, 1 出错
进度: 120/120 已复制, 10 跳过, 1 出错
复制全部完成: 120 个文件处理，10 个跳过，1 个出错
```

**输出说明：**
- **扫描阶段**: 实时显示正在扫描的路径
- **复制进度**: 显示最近复制的源路径和目标路径
- **统计信息**: 显示当前复制进度（已复制/总数，已跳过，出错数）
- **扫描完成**: 当扫描结束后显示此提示，继续等待剩余复制任务
- **最终结果**: 显示完整的复制统计

## 工作原理

1. 从指定的搜索根目录开始递归查找所有包含 `.git` 目录的 Git 仓库
2. 对每个仓库执行 `git ls-files -i --exclude-standard -o -z` 获取被忽略的文件列表
3. 应用用户指定的排除模式过滤文件
4. 对于每个待复制文件，检查目标文件是否存在且更新
5. 使用原子复制（临时文件 + 重命名）确保数据完整性
6. 并行处理多个文件以提高性能

## 要求

- Go 1.21+
- Git (需要在 PATH 中)
- Windows 操作系统

## 测试

运行测试需要 Git 在系统 PATH 中：

```bash
go test ./tests/...
```

## 许可证

遵循 `.cursor/rules/common.mdc` 中的工程规范。
