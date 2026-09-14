@echo off
rem ============================================================
rem  auto-anime - local dev launcher
rem  Usage: double-click; app logs print here, Aria2 logs go to aria2.log.
rem  Every launch rebuilds the exe from current source first.
rem  Exit: close this window, or Ctrl+C then Y -
rem        aria2 and auto-anime exit together.
rem  NOTE: keep this file ASCII-only. cmd.exe under chcp 65001
rem        mis-advances its read position after multi-byte UTF-8
rem        lines (Windows batch bug), randomly truncating later
rem        lines. All messages here are English on purpose; the
rem        web UI itself stays Simplified Chinese.
rem ============================================================
setlocal
title auto-anime - local dev

rem ===== config =====
set "APP_EXE=E:\anime-e2e\auto-anime.exe"
set "GO_EXE=E:\go-sdk\go\bin\go.exe"
set "ARIA2_EXE=E:\go-sdk\aria2\aria2-1.37.0-win-64bit-build1\aria2c.exe"
set "DATA_DIR=E:\anime-e2e"
set "RPC_SECRET=e2e-secret"
set "PORT=8000"
set "ARIA2_PORT=6800"
rem backend dir (Go sources + static UI files) = parent of this script's dir + backend
set "PROJECT_DIR=%~dp0..\backend"

if not exist "%ARIA2_EXE%" (
  echo [ERROR] aria2 not found: %ARIA2_EXE%
  pause
  exit /b 1
)

netstat -ano | findstr ":%PORT% " | findstr /i "LISTENING" >nul && (
  echo [ERROR] port %PORT% already in use - another instance may be running.
  pause
  exit /b 1
)
netstat -ano | findstr ":%ARIA2_PORT% " | findstr /i "LISTENING" >nul && (
  echo [ERROR] port %ARIA2_PORT% already in use - another aria2 may be running.
  pause
  exit /b 1
)

rem static UI files are read from the working directory - cd first, then build
rem retry once: right after mass process kills the path can transiently not resolve
cd /d "%PROJECT_DIR"
if errorlevel 1 (
  ping -n 2 127.0.0.1 >nul
  cd /d "%PROJECT_DIR"
)

rem rebuild from current source on every launch
set "GO_BIN="
if exist "%GO_EXE%" set "GO_BIN=%GO_EXE%"
if not defined GO_BIN where go >nul 2>nul && set "GO_BIN=go"
if defined GO_BIN (
  echo [BUILD] building latest code into %APP_EXE% ...
  "%GO_BIN%" build -o "%APP_EXE%" .
  if errorlevel 1 (
    echo [ERROR] build failed - check Go output above.
    pause
    exit /b 1
  )
  echo [BUILD] done
) else if not exist "%APP_EXE%" (
  echo [ERROR] Go not found - neither %GO_EXE% nor PATH, and %APP_EXE% does not exist.
  pause
  exit /b 1
) else (
  echo [WARN] Go not found - skipping build, using existing %APP_EXE%
)

if not exist "%DATA_DIR%\aria2" mkdir "%DATA_DIR%\aria2"
if not exist "%DATA_DIR%\downloads" mkdir "%DATA_DIR%\downloads"

echo [START] aria2 (RPC %ARIA2_PORT%, BT/DHT on)...
rem Keep task IDs across restarts and verify files whose control record is gone.
set "ARIA2_SESSION=%DATA_DIR%\aria2\session.txt"
set "ARIA2_INPUT="
if exist "%ARIA2_SESSION%" set ARIA2_INPUT=--input-file="%ARIA2_SESSION%"
rem Console selection can block writes and stall RPC; write Aria2 logs to a file.
start "" /b "%ARIA2_EXE%" --enable-rpc --rpc-listen-port=%ARIA2_PORT% --rpc-secret=%RPC_SECRET% --dir="%DATA_DIR%\aria2" --enable-dht=true --dht-listen-port=6881 --listen-port=6881 --bt-enable-lpd=true --seed-ratio=1.0 --seed-time=30 --file-allocation=none --continue=true --check-integrity=true %ARIA2_INPUT% --save-session="%ARIA2_SESSION%" --save-session-interval=10 --auto-save-interval=10 --force-save=true --summary-interval=0 --show-console-readout=false --log="%DATA_DIR%\aria2\aria2.log" --log-level=notice --console-log-level=warn >nul 2>&1

rem env vars for auto-anime (inherited by the child process)
set "DB_PATH=%DATA_DIR%\auto-anime.db"
set "DOWNLOAD_DIR=%DATA_DIR%\downloads"
set "DOWNLOAD_MODE=aria2"
set "ARIA2_URL=http://127.0.0.1:%ARIA2_PORT%/jsonrpc"
set "ARIA2_TOKEN=%RPC_SECRET%"
set "ARIA2_DIR=%DATA_DIR%\aria2"
set "ARIA2_LOCAL_DIR=%DATA_DIR%\aria2"
set "DOWNLOADER_POLL_SECONDS=10"
set "POLL_MINUTES=30"

echo [START] auto-anime (HTTP %PORT%)...
start "" /b "%APP_EXE%"

echo.
echo ==================================================
echo   Web UI    : http://127.0.0.1:%PORT%
echo   Archive   : %DOWNLOAD_DIR%
echo   BT cache  : %DATA_DIR%\aria2 (seeding; archive is a copy)
echo   Aria2 log : %DATA_DIR%\aria2\aria2.log
echo   Exit      : close this window, or Ctrl+C then Y
echo ==================================================
echo.

:waitloop
ping -n 3601 127.0.0.1 >nul
goto waitloop
