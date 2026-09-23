$GoArgs = $args
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$env:GOCACHE = Join-Path $projectRoot '.tools/go-build'
$env:GOMODCACHE = Join-Path $projectRoot '.tools/go-mod'
$env:GOPATH = Join-Path $projectRoot '.tools/go-path'
$env:GOTELEMETRY = 'off'
$env:GOTOOLCHAIN = 'local'
$goTool = Join-Path $projectRoot '.tools/go/bin/go.exe'
if (-not (Test-Path -LiteralPath $goTool)) { $goTool = (Get-Command go -ErrorAction Stop).Source }
& $goTool @GoArgs
exit $LASTEXITCODE
