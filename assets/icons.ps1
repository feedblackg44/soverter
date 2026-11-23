# Regenerates all icons from assets\icon.png: assets\soverter.ico, shellext\logo*.png and the Windows
# resource cmd\soverter\rsrc_windows_amd64.syso (committed, so a plain `go build` embeds the icon).
# Needs windres (MinGW) only here, not for building.
$ErrorActionPreference = 'Stop'
Set-Location "$PSScriptRoot\.."
go run ./cmd/icongen
if ($LASTEXITCODE) { throw 'icongen failed' }
Push-Location assets
windres -F pe-x86-64 -O coff -i soverter.rc -o ..\cmd\soverter\rsrc_windows_amd64.syso
$rc = $LASTEXITCODE
Remove-Item soverter.rc
Pop-Location
if ($rc) { throw 'windres failed' }
