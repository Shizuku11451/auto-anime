@echo off
rem ============================================================
rem  自动追番 - 本地一键测试启动
rem  用法：双击运行；日志直接打印在本窗口。
rem  退出：关闭本窗口，或在窗口内按 Ctrl+C 后回答 Y，
rem        aria2 和 auto-anime 会一起退出。
rem ============================================================
setlocal
chcp 65001 >nul
title 自动追番 - 本地测试

rem ===== 配置区（按需修改） =====
set "APP_EXE=E:\anime-e2e\auto-anime.exe"
set "ARIA2_EXE=E:\go-sdk\aria2\aria2-1.37.0-win-64bit-build1\aria2c.exe"
set "DATA_DIR=E:\anime-e2e"
set "RPC_SECRET=e2e-secret"
set "PORT=8000"
set "ARIA2_PORT=6800"
rem 程序目录即本脚本所在目录（static 界面文件在这里）
set "PROJECT_DIR=%~dp0"

if not exist "%APP_EXE%" (
  echo [错误] 找不到程序 %APP_EXE%
  echo        请先在项目目录执行: go build -o %APP_EXE% .
  pause
  exit /b 1
)
if not exist "%ARIA2_EXE%" (
  echo [错误] 找不到 Aria2 %ARIA2_EXE%
  pause
  exit /b 1
)

netstat -ano | findstr ":%PORT% " | findstr /i "LISTENING" >nul && (
  echo [错误] 端口 %PORT% 已被占用，可能已有实例在运行。
  pause
  exit /b 1
)
netstat -ano | findstr ":%ARIA2_PORT% " | findstr /i "LISTENING" >nul && (
  echo [错误] 端口 %ARIA2_PORT% 已被占用，可能已有 Aria2 在运行。
  pause
  exit /b 1
)

if not exist "%DATA_DIR%\aria2" mkdir "%DATA_DIR%\aria2"
if not exist "%DATA_DIR%\downloads" mkdir "%DATA_DIR%\downloads"

rem 界面静态文件按工作目录读取，必须切到项目目录
cd /d "%PROJECT_DIR"

echo [启动] Aria2（RPC %ARIA2_PORT%，BT/DHT 开启）...
start "" /b "%ARIA2_EXE%" --enable-rpc --rpc-secret=%RPC_SECRET% --dir="%DATA_DIR%\aria2" --enable-dht=true --dht-listen-port=6881 --listen-port=6881 --bt-enable-lpd=true --seed-ratio=1.0 --seed-time=30 --file-allocation=none --continue=true --summary-interval=30

rem 传给 auto-anime 的环境变量（子进程继承）
set "DB_PATH=%DATA_DIR%\auto-anime.db"
set "DOWNLOAD_DIR=%DATA_DIR%\downloads"
set "DOWNLOAD_MODE=aria2"
set "ARIA2_URL=http://127.0.0.1:%ARIA2_PORT%/jsonrpc"
set "ARIA2_TOKEN=%RPC_SECRET%"
set "ARIA2_DIR=%DATA_DIR%\aria2"
set "ARIA2_LOCAL_DIR=%DATA_DIR%\aria2"
set "DOWNLOADER_POLL_SECONDS=10"
set "POLL_MINUTES=30"

echo [启动] auto-anime（HTTP %PORT%）...
start "" /b "%APP_EXE%"

echo.
echo ==================================================
echo   管理页面 : http://127.0.0.1:%PORT%
echo   归档目录 : %DOWNLOAD_DIR%
echo   下载缓存 : %DATA_DIR%\aria2（做种中，归档是复制）
echo   退出方式 : 关闭本窗口，或 Ctrl+C 后回答 Y
echo ==================================================
echo.

:waitloop
ping -n 3601 127.0.0.1 >nul
goto waitloop
