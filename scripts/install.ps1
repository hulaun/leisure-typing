# Builds bt and installs the shim that puts it on the PATH.
#
# %LOCALAPPDATA%\Microsoft\WindowsApps is on the Windows PATH by default, so a
# one-line bt.cmd there makes `bt` work from cmd, PowerShell, Git Bash and the
# VS Code terminal, from any directory — the same arrangement as quick-tools'
# qt.cmd. The binary itself stays in the repo, so a rebuild needs no reinstall.

$ErrorActionPreference = 'Stop'

$repo = Split-Path -Parent $PSScriptRoot
$exe  = Join-Path $repo 'bin\bt.exe'

Write-Host "building $exe"
& go build -o $exe (Join-Path $repo 'cmd\bt')
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

$shimDir = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps'
if (-not (Test-Path $shimDir)) { New-Item -ItemType Directory -Path $shimDir | Out-Null }
$shim = Join-Path $shimDir 'bt.cmd'

# %* forwards the arguments; the redirect-free call keeps stdin and stdout
# attached to the real console, which the reader needs for raw mode and VT.
@"
@echo off
"$exe" %*
"@ | Out-File -FilePath $shim -Encoding ascii

Write-Host "installed $shim -> $exe"
Write-Host ""
Write-Host "open a new terminal, then:  bt import <file>"
