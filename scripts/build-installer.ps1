param(
  [ValidatePattern('^\d+\.\d+\.\d+$')][string]$Version = '0.2.0',
  [string]$CompilerPath = '',
  [switch]$SkipAppBuild
)
$ErrorActionPreference = 'Stop'
$kolRoot = Split-Path $PSScriptRoot -Parent
Push-Location $kolRoot
try {
  if (-not $SkipAppBuild) { & "$PSScriptRoot\build.ps1" -Version $Version }
  if (-not (Test-Path 'dist\KOL.exe')) { throw 'Compilez KOL avant de créer son installateur.' }
  if (-not $CompilerPath) {
    $kolCommand = Get-Command ISCC.exe -ErrorAction SilentlyContinue
    if ($kolCommand) { $CompilerPath = $kolCommand.Source }
  }
  if (-not $CompilerPath) {
    foreach ($kolCandidate in @(
      "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
      "$env:ProgramFiles\Inno Setup 7\ISCC.exe",
      "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
    )) {
      if (Test-Path $kolCandidate) { $CompilerPath = $kolCandidate; break }
    }
  }
  if (-not $CompilerPath) { throw 'Inno Setup 6.7 ou plus récent est nécessaire pour créer l’installateur.' }
  & $CompilerPath "/DAppVersion=$Version" "/O$kolRoot\dist" 'installer\KOL.iss'
  if ($LASTEXITCODE -ne 0) { throw 'Création de l’installateur KOL impossible.' }
  Write-Output "Installateur créé : $kolRoot\dist\KOL-Setup-$Version.exe"
} finally { Pop-Location }
