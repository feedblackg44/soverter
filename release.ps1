# Builds a release: the signed Windows 11 menu package, then the GUI exe with the package embedded,
# and puts soverter.exe + a zip into dist\ (upload them to a GitHub release; dist\ is not committed).
#
# Self-signed (default) or a real code-signing certificate:
#   .\release.ps1
#   .\release.ps1 -Thumbprint <sha1> -TimestampUrl http://timestamp.digicert.com
param(
    [string]$Version = '1.0.0',
    [string]$Thumbprint,
    [string]$TimestampUrl
)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$pkgArgs = @{ Version = "$Version.0" }
if ($Thumbprint) { $pkgArgs.Thumbprint = $Thumbprint }
if ($TimestampUrl) { $pkgArgs.TimestampUrl = $TimestampUrl }
& "$PSScriptRoot\shellext\package.ps1" @pkgArgs

New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -ldflags "-H=windowsgui -s -w" -o dist\soverter.exe .\cmd\soverter
if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
Compress-Archive -Force -Path dist\soverter.exe -DestinationPath "dist\soverter-$Version-windows-x64.zip"
Get-ChildItem dist | Select-Object Name, Length
