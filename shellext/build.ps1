# Builds the Windows 11 context-menu DLL and the package stub into shellext\build.
# Requires MinGW-w64 (g++/gcc) in PATH. Normally invoked by package.ps1.
$ErrorActionPreference = 'Stop'
$out = Join-Path $PSScriptRoot 'build'
New-Item -ItemType Directory -Force $out | Out-Null
g++ -shared -O2 -s -static -static-libgcc -static-libstdc++ -o "$out\vidpreset_ctx.dll" "$PSScriptRoot\ctx.cpp" "$PSScriptRoot\ctx.def" -lole32 -lshell32 -lshlwapi -luuid
if ($LASTEXITCODE) { throw 'dll build failed' }
gcc -O2 -s -o "$out\stub.exe" "$PSScriptRoot\stub.c"
if ($LASTEXITCODE) { throw 'stub build failed' }
