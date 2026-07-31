@echo off
chcp 65001 >nul
cd /d "%~dp0"

if not exist output mkdir output
if not exist bin mkdir bin

echo Building nieoserver ...
go build -ldflags "-s -w" -o output\nieoserver.exe .\cmd\nieoserver
if %errorlevel% neq 0 (
    echo BUILD FAILED: nieoserver
    pause
    exit /b 1
)

echo Building logintest ...
go build -ldflags "-s -w" -o output\logintest.exe .\cmd\logintest
if %errorlevel% neq 0 (
    echo BUILD FAILED: logintest
    pause
    exit /b 1
)

echo.
echo Copy to bin\ (close running server first if copy fails)
copy /y output\nieoserver.exe bin\nieoserver.exe
if %errorlevel% neq 0 (
    echo.
    echo COPY FAILED: close nieoserver.exe then run build.bat again
    pause
    exit /b 1
)
copy /y output\logintest.exe bin\logintest.exe >nul

echo.
echo BUILD OK
echo   - output\nieoserver.exe
echo   - bin\nieoserver.exe   (start.bat runs this)
echo   - output\logintest.exe
echo   - bin\logintest.exe
pause
