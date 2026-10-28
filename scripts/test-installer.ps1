param([Parameter(Mandatory = $true)][string]$InstallerPath)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows') {
  throw 'Ce test installe puis désinstalle KOL : il est réservé au runner Windows jetable de la CI.'
}
$kolUninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{B7C6A102-5E4F-4D85-92E0-1B1DC54EB923}_is1'
$kolAppPathsKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\KOL.exe'
$kolRunKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
if ((Test-Path $kolUninstallKey) -or (Test-Path $kolAppPathsKey)) { throw 'Une installation KOL existe déjà : test annulé.' }
$kolScratch = Join-Path $env:RUNNER_TEMP ('kol-installer-' + [guid]::NewGuid().ToString('N'))
$kolApp = Join-Path $kolScratch 'Application KOL'
$kolData = Join-Path $kolScratch 'Historique conservé'
New-Item -ItemType Directory -Path $kolData -Force | Out-Null
Set-Content (Join-Path $kolData 'settings.json') '{"startAtLogin":false,"maxItemMiB":512}' -Encoding utf8NoBOM
[IO.File]::WriteAllBytes((Join-Path $kolData 'history.db'), [byte[]](1, 2, 3, 4, 255))
[IO.File]::WriteAllBytes((Join-Path $kolData 'history.key'), [byte[]](9, 8, 7, 6, 0))
$kolBeforeDB = (Get-FileHash (Join-Path $kolData 'history.db')).Hash
$kolBeforeKey = (Get-FileHash (Join-Path $kolData 'history.key')).Hash

function Get-KolStartup {
  $kolKey = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Run')
  if (-not $kolKey) { return $null }
  try { return $kolKey.GetValue('KOL', $null) } finally { $kolKey.Dispose() }
}
function Invoke-KolSetup([string]$TaskArgument = '') {
  $kolLog = Join-Path $kolScratch ('setup-' + [guid]::NewGuid().ToString('N') + '.log')
  $kolArguments = "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /DIR=`"$kolApp`" /DataDir=`"$kolData`" /LOG=`"$kolLog`" $TaskArgument"
  $kolProcess = Start-Process -FilePath $InstallerPath -ArgumentList $kolArguments -Wait -PassThru
  if ($kolProcess.ExitCode -ne 0) {
    Get-Content $kolLog -Tail 50
    throw "L'installation a échoué : $($kolProcess.ExitCode)"
  }
}
function Assert-KolDataPreserved {
  if ((Get-FileHash (Join-Path $kolData 'history.db')).Hash -ne $kolBeforeDB) { throw 'Historique modifié par l''installation.' }
  if ((Get-FileHash (Join-Path $kolData 'history.key')).Hash -ne $kolBeforeKey) { throw 'Clé modifiée par l''installation.' }
  $kolSettings = Get-Content (Join-Path $kolData 'settings.json') -Raw | ConvertFrom-Json
  if ($kolSettings.maxItemMiB -ne 512) { throw 'Préférences perdues.' }
}

Invoke-KolSetup '/TASKS=startup'
if (-not (Test-Path (Join-Path $kolApp 'KOL.exe'))) { throw 'Exécutable non installé.' }
if ((Get-ItemProperty $kolUninstallKey).DisplayName -ne 'KOL') { throw 'KOL absent des applications installées.' }
if ((Get-Item $kolAppPathsKey).GetValue('') -ne (Join-Path $kolApp 'KOL.exe')) { throw 'Enregistrement App Paths incorrect.' }
$kolStartMenu = Join-Path ([Environment]::GetFolderPath('Programs')) 'KOL.lnk'
if (-not (Test-Path $kolStartMenu)) { throw 'Raccourci du menu Démarrer absent.' }
$kolStartup = Get-KolStartup
if (-not $kolStartup.StartsWith('"' + (Join-Path $kolApp 'KOL.exe') + '"')) { throw 'Le démarrage ne vise pas la copie installée.' }
if ((Get-Content (Join-Path $kolData 'settings.json') -Raw | ConvertFrom-Json).startAtLogin -ne $true) { throw 'Préférence de démarrage non synchronisée.' }
Assert-KolDataPreserved

# Opening the uninstaller and cancelling must not disable a retained app.
Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public static class KolInstallerConfirmation {
  private delegate bool EnumProc(IntPtr window, IntPtr unused);
  [DllImport("user32.dll")] private static extern bool EnumWindows(EnumProc callback, IntPtr unused);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetWindowText(IntPtr window, StringBuilder text, int count);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetClassName(IntPtr window, StringBuilder text, int count);
  [DllImport("user32.dll")] private static extern IntPtr GetDlgItem(IntPtr window, int id);
  [DllImport("user32.dll")] private static extern bool PostMessage(IntPtr window, uint message, IntPtr wparam, IntPtr lparam);
  public static bool Cancel() {
    bool result = false;
    EnumWindows((window, unused) => {
      var title = new StringBuilder(256); var kind = new StringBuilder(64);
      GetWindowText(window, title, title.Capacity); GetClassName(window, kind, kind.Capacity);
      if (title.ToString().IndexOf("KOL", StringComparison.OrdinalIgnoreCase) < 0 || kind.ToString() != "#32770") return true;
      var no = GetDlgItem(window, 7); if (no == IntPtr.Zero) return true;
      result = PostMessage(window, 0x111, (IntPtr)7, no); return false;
    }, IntPtr.Zero);
    return result;
  }
}
'@
$kolCancelProcess = Start-Process -FilePath (Join-Path $kolApp 'unins000.exe') -PassThru
$kolCancelled = $false
$kolUntil = [DateTime]::UtcNow.AddSeconds(20)
while ([DateTime]::UtcNow -lt $kolUntil) {
  if ([KolInstallerConfirmation]::Cancel()) { $kolCancelled = $true; break }
  Start-Sleep -Milliseconds 100
}
if (-not $kolCancelled) { throw 'Confirmation de désinstallation introuvable.' }
if (-not $kolCancelProcess.WaitForExit(10000)) { throw 'Annulation de désinstallation bloquée.' }
if (-not (Test-Path $kolUninstallKey)) { throw 'Installation supprimée malgré une annulation.' }
if ((Get-KolStartup) -ne $kolStartup) { throw 'Une annulation a changé le démarrage automatique.' }
if (-not (Get-Content (Join-Path $kolData 'settings.json') -Raw | ConvertFrom-Json).startAtLogin) { throw 'Une annulation a désactivé le démarrage.' }
Assert-KolDataPreserved

# Upgrade without explicit tasks must retain the user's enabled startup choice.
Invoke-KolSetup
if (-not (Get-KolStartup)) { throw 'Démarrage perdu lors de la mise à jour.' }
Assert-KolDataPreserved

Invoke-KolSetup '/MERGETASKS=desktopicon'
if (-not (Get-KolStartup)) { throw 'Un autre raccourci a désactivé le démarrage.' }
Assert-KolDataPreserved

Invoke-KolSetup '/MERGETASKS=!startup'
if (Get-KolStartup) { throw 'La demande explicite de désactivation a été ignorée.' }
Invoke-KolSetup '/MERGETASKS=startup'
if (-not (Get-KolStartup)) { throw 'La demande explicite de démarrage a été ignorée.' }
Assert-KolDataPreserved

# Emulate the user turning startup off in KOL, then update again.
$kolConfig = Start-Process -FilePath (Join-Path $kolApp 'KOL.exe') -ArgumentList "--configure-startup=disable --data-dir `"$kolData`"" -Wait -PassThru
if ($kolConfig.ExitCode -ne 0) { throw 'Désactivation du démarrage impossible.' }
Invoke-KolSetup
if (Get-KolStartup) { throw 'La mise à jour a réactivé le démarrage.' }
Assert-KolDataPreserved

# Uninstall while startup is enabled, proving its entry is removed too.
$kolConfig = Start-Process -FilePath (Join-Path $kolApp 'KOL.exe') -ArgumentList "--configure-startup=enable --data-dir `"$kolData`"" -Wait -PassThru
if ($kolConfig.ExitCode -ne 0) { throw 'Préparation de la désinstallation impossible.' }
$kolUninstaller = Join-Path $kolApp 'unins000.exe'
$kolProcess = Start-Process -FilePath $kolUninstaller -ArgumentList '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART' -Wait -PassThru
if ($kolProcess.ExitCode -ne 0) { throw 'Désinstallation impossible.' }
if (Test-Path $kolUninstallKey) { throw 'Enregistrement de désinstallation conservé.' }
if (Test-Path $kolAppPathsKey) { throw 'App Paths non nettoyé.' }
if (Test-Path (Join-Path $kolApp 'KOL.exe')) { throw 'Exécutable non supprimé.' }
if (Test-Path $kolStartMenu) { throw 'Raccourci non supprimé.' }
if (Get-KolStartup) { throw 'Démarrage automatique non supprimé.' }
if ((Get-Content (Join-Path $kolData 'settings.json') -Raw | ConvertFrom-Json).startAtLogin) { throw 'Démarrage encore activé dans les préférences.' }
Assert-KolDataPreserved
Write-Output 'PASS : installation, raccourci, enregistrement Windows, mise à jour, démarrage et désinstallation ; historique, clé et préférences conservés.'
