@echo off
setlocal enabledelayedexpansion
title Spin Studio HUD Launcher

echo ========================================================
echo          Spin Studio HUD - Telemetry Player
echo ========================================================
echo.

cd /d "%~dp0"

:: 1. Prefer compiled Go binary if present
if exist "%~dp0spin-hud.exe" (
    echo [INFO] Launching Spin Studio (compiled native)...
    "%~dp0spin-hud.exe" %*
    if !ERRORLEVEL! neq 0 (
        echo.
        echo [INFO] Spin Studio exited with code !ERRORLEVEL!.
        pause
    )
    exit /b !ERRORLEVEL!
)

:: 2. Detect Python fallback
where python >nul 2>nul
if %ERRORLEVEL% equ 0 (
    set "PY_CMD=python"
    goto :check_deps
)

where py >nul 2>nul
if %ERRORLEVEL% equ 0 (
    set "PY_CMD=py"
    goto :check_deps
)

echo [ERROR] Python was not found on your system!
echo Please install Python 3.10+ from https://www.python.org/downloads/
echo Make sure to check "Add Python to PATH" during installation.
echo.
pause
exit /b 1

:check_deps
:: 3. Check and auto-install bleak dependency
%PY_CMD% -c "import bleak" >nul 2>nul
if errorlevel 1 (
    echo [INFO] Installing required Bluetooth library bleak...
    %PY_CMD% -m pip install "bleak>=0.21.0"
    if errorlevel 1 (
        echo [ERROR] Failed to install bleak. Please check your internet connection.
        pause
        exit /b 1
    )
    echo [INFO] Dependencies installed successfully!
    echo.
)

:: 4. Launch Spin Studio Python
echo [INFO] Launching Spin Studio Web HUD and BLE Engine...
%PY_CMD% "%~dp0spin_hud.py" %*

if %ERRORLEVEL% neq 0 (
    echo.
    echo [INFO] Spin Studio exited with code %ERRORLEVEL%.
    pause
)
