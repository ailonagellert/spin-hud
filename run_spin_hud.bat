@echo off
:: Delegate to standardized launcher which prefers native compiled Go binary, falling back to Python
call "%~dp0launch.bat" %*
exit /b %ERRORLEVEL%

