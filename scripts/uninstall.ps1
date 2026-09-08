# Removes the bt shim. The books in storage/ are left alone.
$ErrorActionPreference = 'Stop'
$shim = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\bt.cmd'
if (Test-Path $shim) {
    Remove-Item $shim
    Write-Host "removed $shim"
} else {
    Write-Host "no shim at $shim"
}
