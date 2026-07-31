@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo ========================================
echo  Nieo backend one-click start
echo  GM=29180 RES=41520 LOGIN=22973 GAME=28610
echo  MySQL root@127.0.0.1/niao
echo ========================================

if not exist "configs\advertise_host.txt" (
  echo 127.0.0.1> "configs\advertise_host.txt"
)

echo [patch] ServerR.xml login IP/port ...
powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\patch_serverr_local.ps1"
if errorlevel 1 (
  echo [warn] ServerR patch failed
)

echo [sync] tmp\dll -^> NieoData\dll ...
if not exist "..\NieoData\dll" mkdir "..\NieoData\dll"
if exist "..\tmp\dll\NieoCore.swf" (
  copy /Y "..\tmp\dll\*" "..\NieoData\dll\" >nul
  echo [ok] NieoCore.swf ready
) else (
  echo [warn] missing ..\tmp\dll\NieoCore.swf
)
if exist "..\tmp\core.version" (
  copy /Y "..\tmp\core.version" "..\NieoData\core.version" >nul
  echo [ok] core.version ready
)

if not exist "bin\nieoserver.exe" (
  echo [build] bin\nieoserver.exe missing, compiling ...
  if not exist output mkdir output
  if not exist bin mkdir bin
  go build -ldflags "-s -w" -o output\nieoserver.exe .\cmd\nieoserver
  if errorlevel 1 (
    echo [fail] build failed
    pause
    exit /b 1
  )
  copy /y output\nieoserver.exe bin\nieoserver.exe >nul
  echo [ok] built bin\nieoserver.exe
)

echo [hint] MySQL must be running
echo [run] GM  http://127.0.0.1:29180/
echo [run] RES http://127.0.0.1:41520/
echo [log] [RES]=resource  [CMD] OK/UNIMPL=protocol
echo.
bin\nieoserver.exe -root "%cd%"
pause
