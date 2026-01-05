@echo off
REM Windows 构建脚本

echo 构建 copy-ignore Windows exe...

REM 标准构建
echo 构建标准版本...
go build -o copy-ignore.exe main.go
if %errorlevel% neq 0 (
    echo 构建失败
    exit /b 1
)

REM 优化构建
echo 构建优化版本...
go build -ldflags="-s -w" -o copy-ignore-release.exe main.go
if %errorlevel% neq 0 (
    echo 构建失败
    exit /b 1
)

echo 构建完成！
echo.
echo 文件信息:
dir *.exe
echo.
echo 测试运行:
copy-ignore-release.exe --help

echo.
echo 构建脚本执行完毕。
