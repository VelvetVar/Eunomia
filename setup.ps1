[CmdletBinding()]
param(
    [string]$InstallRoot = $env:EUNOMIA_INSTALL_ROOT,
    [string]$BinDir = $env:EUNOMIA_BIN_DIR,
    [switch]$NoPath,
    [switch]$SkipSystemChanges,
    [switch]$Offline
)
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'Run sh ./setup.sh on Linux or macOS.' }
foreach ($directory in @($InstallRoot, $BinDir)) {
    if ($directory -and ($directory -notmatch '^(?:[A-Za-z]:[\\/]|\\\\[^\\/]+[\\/][^\\/]+)' -or $directory -match '[\r\n]')) {
        throw 'Install and command directories must be absolute paths without newlines.'
    }
}
$architecture = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $architecture = $env:PROCESSOR_ARCHITEW6432 }
$arch = switch ($architecture) { 'AMD64' { 'amd64' } 'ARM64' { 'arm64' } default { throw 'This package supports Windows x64 and ARM64.' } }
$binary = Join-Path $PSScriptRoot "bin/windows-$arch/eunomia.exe"
if (-not (Test-Path -LiteralPath $binary)) {
    $binary = Join-Path $PSScriptRoot 'eunomia.exe'
    if (-not (Test-Path -LiteralPath $binary)) {
        $goCommand = Get-Command go -ErrorAction SilentlyContinue
        if (-not $goCommand) { throw 'Extract the complete release package, or install Go 1.26+ and run: go build -o eunomia.exe ./cmd/eunomia' }
        Push-Location $PSScriptRoot
        $savedGoEnvironment = @{}
        try {
            if ($Offline) {
                foreach ($name in @('GOPROXY','GOSUMDB','GOTOOLCHAIN')) { $savedGoEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
                $env:GOPROXY = 'off'; $env:GOSUMDB = 'off'; $env:GOTOOLCHAIN = 'local'
            }
            & $goCommand.Source build -trimpath -o $binary ./cmd/eunomia
            if ($LASTEXITCODE -ne 0) { throw 'Go build failed. Offline builds require a locally installed toolchain and cached dependencies.' }
        }
        finally {
            foreach ($name in $savedGoEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $savedGoEnvironment[$name], 'Process') }
            Pop-Location
        }
    }
}
if (Test-Path -LiteralPath ($binary + '.sha256')) {
    $expected = ((Get-Content -LiteralPath ($binary + '.sha256') -Raw).Trim() -split '\s+')[0]
    if ((Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash -ine $expected) { throw 'Executable checksum mismatch. Re-extract the package.' }
}
$report = (& $binary doctor --json | Out-String) | ConvertFrom-Json
$missingSsh = @($report.checks | Where-Object { ($_.id -eq 'ssh' -or $_.id -eq 'ssh-keygen') -and $_.status -ne 'ok' }).Count -gt 0
if ($missingSsh -and -not $SkipSystemChanges -and -not $Offline) {
    Write-Host 'Installing the OpenSSH Client optional feature. Windows may request administrator approval.'
    $command = '$ErrorActionPreference = "Stop"; try { Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0 | Out-Null; exit 0 } catch { Write-Error $_; exit 1 }'
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($command))
    $powershell = Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe'
    $process = Start-Process -FilePath $powershell -ArgumentList @('-NoProfile','-EncodedCommand',$encoded) -Verb RunAs -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw 'OpenSSH installation failed. Enable the OpenSSH Client feature manually and rerun setup.' }
}
$arguments = @('install')
if ($InstallRoot) { $arguments += @('--root', $InstallRoot) }
if ($BinDir) { $arguments += @('--bin', $BinDir) }
if ($NoPath) { $arguments += '--no-path' }
& $binary @arguments
if ($LASTEXITCODE -ne 0) { throw 'Setup did not complete. Resolve the error above and rerun setup.' }
