package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ListIgnoredFiles 使用 git ls-files 命令列出指定仓库中被忽略的文件
// 返回相对于仓库根目录的相对路径列表
func ListIgnoredFiles(repoRoot string) ([]string, error) {
	// 使用 git ls-files -i --exclude-standard -o -z 列出被忽略的未追踪文件
	// -i: 显示被忽略的文件
	// --exclude-standard: 使用标准的忽略规则（包括 .gitignore）
	// -o: 显示未被追踪的文件（与 -i 一起使用时显示被忽略的未追踪文件）
	// -z: 以 null 字符分隔输出，避免路径中空格的问题
	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "-i", "--exclude-standard", "-o", "-z")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("执行 git ls-files 失败: %v\n错误输出: %s", err, stderr.String())
	}

	// 解析 null 分隔的输出
	output := stdout.Bytes()
	if len(output) == 0 {
		return []string{}, nil
	}

	// 使用 null 字符分割（最后一个元素是空字符串，需要去掉）
	parts := bytes.Split(output, []byte{0})
	files := make([]string, 0, len(parts)-1)

	for _, part := range parts {
		if len(part) > 0 {
			// 转换为字符串并清理路径
			file := string(part)
			file = filepath.Clean(file)

			// 跳过空字符串和无效路径
			if file != "" && file != "." && file != ".." {
				files = append(files, file)
			}
		}
	}

	return files, nil
}

// IsGitRepository 检查指定目录是否为 Git 仓库
func IsGitRepository(dir string) bool {
	// 检查 .git 目录是否存在
	gitDir := filepath.Join(dir, ".git")
	if info, err := exec.Command("git", "-C", dir, "rev-parse", "--git-dir").Output(); err == nil {
		// git rev-parse 返回的路径可能需要解析
		gitDirFromCmd := strings.TrimSpace(string(info))
		if gitDirFromCmd != "" {
			gitDir = filepath.Join(dir, gitDirFromCmd)
		}
	}

	// 简单检查 .git 目录或文件是否存在
	info, err := exec.Command("cmd", "/c", "if exist \""+gitDir+"\" echo exists").Output()
	return err == nil && strings.TrimSpace(string(info)) == "exists"
}
