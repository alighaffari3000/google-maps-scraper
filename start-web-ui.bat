@echo off
setlocal
cd /d "%~dp0"

set PORT=8081
set EXE=gmaps-scraper.exe

echo ==========================================
echo   Google Maps Scraper - Web UI
echo ==========================================
echo.

rem Port 8080 is taken by another program on this machine, so the UI lives on 8081.

rem Rebuild first so the running server always matches the current source.
rem Windows refuses to overwrite a running .exe, hence the shutdown above it.
taskkill /F /IM %EXE% >nul 2>&1

where go >nul 2>&1
if %ERRORLEVEL%==0 (
    echo Building %EXE% ...
    go build -o %EXE% .
    if errorlevel 1 (
        echo.
        echo Build FAILED - see the errors above.
        pause
        exit /b 1
    )
    echo Build OK.
) else (
    echo Go not found on PATH - using the existing %EXE%.
)

if not exist "%EXE%" (
    echo.
    echo %EXE% is missing and Go is not installed, so it cannot be built.
    pause
    exit /b 1
)

echo.
echo Starting server on http://localhost:%PORT%
start "" http://localhost:%PORT%
%EXE% -web -data-folder webdata -addr :%PORT%

echo.
echo Server stopped.
pause
