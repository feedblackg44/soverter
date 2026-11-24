# Builds and signs internal\shellext\assets\soverter.msix (embedded into soverter.exe).
#
# Default: uses (or creates) a self-signed certificate "CN=soverter" in Cert:\CurrentUser\My and exports its
# public part to assets\soverter.cer; the app then installs that .cer into LocalMachine\TrustedPeople (one UAC prompt).
#
# For a public release pass a real code-signing certificate; no trust step is needed on user machines then
# and assets\soverter.cer is not produced:
#   .\package.ps1 -Thumbprint <sha1 of cert in CurrentUser\My> -TimestampUrl http://timestamp.digicert.com
#   .\package.ps1 -Pfx cert.pfx -PfxPassword ... -TimestampUrl ...
param(
    [string]$Subject = 'CN=soverter',
    [string]$Thumbprint,
    [string]$Pfx,
    [string]$PfxPassword,
    [string]$TimestampUrl,
    [string]$Version = '1.0.0.0',
    # Build the package unsigned for an external signing service. -Publisher must equal the
    # Subject of the certificate the service signs with. No .cer is produced, so users get no trust step.
    [switch]$Unsigned,
    [string]$Publisher
)
$ErrorActionPreference = 'Stop'
$here = $PSScriptRoot
$assets = Join-Path $here '..\internal\shellext\assets'

& "$here\build.ps1"

# --- SDK tools ---
$kits = "${env:ProgramFiles(x86)}\Windows Kits\10\bin"
$bin = Get-ChildItem $kits -Directory | Where-Object { Test-Path "$($_.FullName)\x64\makeappx.exe" } | Sort-Object Name -Descending | Select-Object -First 1
if (-not $bin) { throw 'Windows SDK (makeappx/signtool) not found: winget install Microsoft.WindowsSDK.10.0.22621' }
$makeappx = "$($bin.FullName)\x64\makeappx.exe"
$signtool = "$($bin.FullName)\x64\signtool.exe"

# --- certificate ---
if ($Unsigned) {
    if (-not $Publisher) { throw '-Unsigned needs -Publisher (the Subject of the signing certificate)' }
    $cert = $null
    $publisherSubject = $Publisher
    $selfSigned = $false
    Write-Host "Unsigned package for $Publisher (to be signed externally)"
} elseif ($Pfx) {
    $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2($Pfx, $PfxPassword)
    $signArgs = @('/f', $Pfx) + $(if ($PfxPassword) { @('/p', $PfxPassword) } else { @() })
} else {
    if ($Thumbprint) {
        $cert = Get-Item "Cert:\CurrentUser\My\$Thumbprint"
    } else {
        $cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Subject -eq $Subject -and $_.HasPrivateKey -and $_.NotAfter -gt (Get-Date).AddDays(30) } | Select-Object -First 1
        if (-not $cert) {
            Write-Host "Creating self-signed certificate $Subject"
            $cert = New-SelfSignedCertificate -Type Custom -Subject $Subject -KeyUsage DigitalSignature -FriendlyName 'Soverter package signing' `
                -CertStoreLocation Cert:\CurrentUser\My -NotAfter (Get-Date).AddYears(10) `
                -TextExtension @('2.5.29.37={text}1.3.6.1.5.5.7.3.3', '2.5.29.19={text}')
        }
    }
    $signArgs = @('/sha1', $cert.Thumbprint, '/s', 'My')
}
if (-not $Unsigned) {
    $publisherSubject = $cert.Subject
    $selfSigned = $cert.Subject -eq $cert.Issuer
    Write-Host "Signing as $publisherSubject (self-signed: $selfSigned)"
}

# --- stage ---
$stage = Join-Path $here 'build\stage'
Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $stage | Out-Null
Copy-Item "$here\build\soverter_ctx.dll", "$here\build\stub.exe", "$here\logo.png", "$here\logo44.png", "$here\logo50.png" $stage
(Get-Content "$assets\AppxManifest.xml" -Raw -Encoding UTF8).Replace('@PUBLISHER@', $publisherSubject).Replace('@VERSION@', $Version) |
    Set-Content "$stage\AppxManifest.xml" -Encoding UTF8

# --- pack + sign ---
$msix = Join-Path $assets 'soverter.msix'
& $makeappx pack /o /d $stage /p $msix | Out-Null
if ($LASTEXITCODE) { throw 'makeappx failed' }
$ts = if ($TimestampUrl) { @('/tr', $TimestampUrl, '/td', 'SHA256') } else { @() }
if (-not $Unsigned) {
    & $signtool sign /fd SHA256 @signArgs @ts $msix
    if ($LASTEXITCODE) { throw 'signtool failed' }
}

# --- public certificate for self-signed builds ---
$cer = Join-Path $assets 'soverter.cer'
if ($selfSigned) { Export-Certificate -Cert $cert -FilePath $cer -Type CERT | Out-Null } else { Remove-Item $cer -ErrorAction SilentlyContinue }
Get-ChildItem $assets | Select-Object Name, Length
