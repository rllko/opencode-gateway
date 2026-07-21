# Build opencode-gateway.exe (Windows tray app). Run on Windows with Go installed.
#
#   .\build.ps1            build opencode-gateway.exe
#   .\build.ps1 -Deploy    build, then copy to $env:USERPROFILE\opencode-gateway
#
param([switch]$Deploy)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$env:CGO_ENABLED = '0'

Write-Host '==> Windows tray exe'
go build -tags tray -ldflags '-H=windowsgui -s -w' -o opencode-gateway.exe .\cmd\opencode-gateway
Write-Host ('    opencode-gateway.exe  {0:N0} bytes' -f (Get-Item opencode-gateway.exe).Length)

if ($Deploy) {
	$dst = Join-Path $env:USERPROFILE 'opencode-gateway'
	New-Item -ItemType Directory -Force -Path $dst | Out-Null
	$target = Join-Path $dst 'opencode-gateway.exe'
	try {
		Copy-Item opencode-gateway.exe $target -Force
		Write-Host "==> deployed to $target"
	} catch {
		Copy-Item opencode-gateway.exe (Join-Path $dst 'opencode-gateway-new.exe') -Force
		Write-Host '==> exe is locked (tray app running).'
		Write-Host '    Staged as opencode-gateway-new.exe — quit the tray app, then replace.'
	}
}

Write-Host 'Done.'
