@echo off
setlocal
set "eunomia_root=%~dp0"
if exist "%~dp0..\go.mod" set "eunomia_root=%~dp0..\"
if exist "%eunomia_root%dist\eunomia.exe" (
  "%eunomia_root%dist\eunomia.exe" %*
  exit /b
)
if exist "%eunomia_root%eunomia.exe" (
  "%eunomia_root%eunomia.exe" %*
  exit /b
)
echo Build from the repository root: go build -o dist/eunomia.exe ./cmd/eunomia
echo Or extract the Windows release package and run setup.cmd.
exit /b 1
