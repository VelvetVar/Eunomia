@echo off
if exist "%~dp0eunomia.exe" (
  "%~dp0eunomia.exe" %*
  exit /b
)
echo Build the Go executable first: go build -o eunomia.exe ./cmd/eunomia
echo Or extract the Windows release package and run setup.cmd.
exit /b 1
