#ifndef AppVersion
  #define AppVersion "0.2.0"
#endif

[Setup]
AppId={{B7C6A102-5E4F-4D85-92E0-1B1DC54EB923}
AppName=KOL
AppVersion={#AppVersion}
AppVerName=KOL {#AppVersion}
AppPublisher=nRudzy
AppPublisherURL=https://github.com/nRudzy/KOL
AppSupportURL=https://github.com/nRudzy/KOL/issues
AppUpdatesURL=https://github.com/nRudzy/KOL/releases
DefaultDirName={localappdata}\Programs\KOL
DefaultGroupName=KOL
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
SourceDir=..
OutputDir=dist
OutputBaseFilename=KOL-Setup-{#AppVersion}
SetupIconFile=assets\kol.ico
UninstallDisplayIcon={app}\KOL.exe
UninstallDisplayName=KOL
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
DisableProgramGroupPage=yes
DisableDirPage=auto
DirExistsWarning=no
UsePreviousTasks=no
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

[Languages]
Name: "french"; MessagesFile: "compiler:Languages\French.isl"

[Tasks]
Name: "startup"; Description: "Démarrer KOL avec Windows"; GroupDescription: "Au quotidien :"; Flags: unchecked
Name: "desktopicon"; Description: "Créer un raccourci sur le Bureau"; GroupDescription: "Raccourcis :"; Flags: unchecked

[Files]
; The temporary helper only requests a graceful exit; it never opens history.
Source: "dist\KOL.exe"; Flags: dontcopy
Source: "dist\KOL.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\KOL"; Filename: "{app}\KOL.exe"; Parameters: "--data-dir {code:QuotedDataDir}"; WorkingDir: "{app}"; AppUserModelID: "nRudzy.KOL"
Name: "{autodesktop}\KOL"; Filename: "{app}\KOL.exe"; Parameters: "--data-dir {code:QuotedDataDir}"; WorkingDir: "{app}"; AppUserModelID: "nRudzy.KOL"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\App Paths\KOL.exe"; ValueType: string; ValueName: ""; ValueData: "{app}\KOL.exe"; Flags: uninsdeletekey
Root: HKCU; Subkey: "Software\nRudzy\KOL"; ValueType: string; ValueName: "DataDirectory"; ValueData: "{code:GetDataDir}"; Flags: uninsdeletekey

[Run]
Filename: "{app}\KOL.exe"; Parameters: "--data-dir {code:QuotedDataDir}"; Description: "Ouvrir KOL"; Flags: nowait postinstall skipifsilent; Check: ConfigurationSucceeded

[Code]
var
  SelectedDataDir: String;
  ConfigurationFailed: Boolean;

function GetDataDir(Param: String): String;
begin
  if SelectedDataDir = '' then begin
    SelectedDataDir := ExpandConstant('{param:DataDir|}');
    if SelectedDataDir = '' then
      RegQueryStringValue(HKCU, 'Software\nRudzy\KOL', 'DataDirectory', SelectedDataDir);
    if SelectedDataDir = '' then
      SelectedDataDir := ExpandConstant('{localappdata}\KOL');
    SelectedDataDir := ExpandFileName(SelectedDataDir);
  end;
  Result := SelectedDataDir;
end;

function QuotedDataDir(Param: String): String;
begin
  { Ending in dot also quotes a drive-root data directory correctly. }
  Result := AddQuotes(AddBackslash(GetDataDir('')) + '.');
end;

function ExplicitTaskSelection: Boolean;
var
  I: Integer;
  Arg: String;
begin
  Result := False;
  for I := 1 to ParamCount do begin
    Arg := Uppercase(ParamStr(I));
    if (Pos('/TASKS=', Arg) = 1) or (Pos('/MERGETASKS=', Arg) = 1) then begin
      Result := True;
      Exit;
    end;
  end;
end;

procedure InitializeWizard;
begin
  if not ExplicitTaskSelection and
     RegValueExists(HKCU, 'Software\Microsoft\Windows\CurrentVersion\Run', 'KOL') then
    WizardSelectTasks('startup');
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ExitCode: Integer;
begin
  Result := '';
  ExtractTemporaryFile('KOL.exe');
  if not Exec(ExpandConstant('{tmp}\KOL.exe'), '--quit', '', SW_HIDE,
              ewWaitUntilTerminated, ExitCode) then
    Result := 'Impossible de préparer KOL. Fermez KOL depuis son icône, puis réessayez.'
  else if ExitCode <> 0 then
    Result := 'KOL est encore ouvert. Quittez-le depuis son icône, puis réessayez.';
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Mode: String;
  ExitCode: Integer;
begin
  if CurStep = ssPostInstall then begin
    Mode := 'disable';
    if WizardIsTaskSelected('startup') then Mode := 'enable';
    ConfigurationFailed := not Exec(ExpandConstant('{app}\KOL.exe'),
      '--configure-startup=' + Mode + ' --data-dir ' + QuotedDataDir(''), '',
      SW_HIDE, ewWaitUntilTerminated, ExitCode);
    if not ConfigurationFailed then ConfigurationFailed := ExitCode <> 0;
    if ConfigurationFailed then
      SuppressibleMsgBox('KOL a été copié, mais ses préférences de démarrage n''ont pas pu être enregistrées. Relancez l''installation.', mbError, MB_OK, IDOK);
  end;
end;

function ConfigurationSucceeded: Boolean;
begin
  Result := not ConfigurationFailed;
end;

function GetCustomSetupExitCode: Integer;
begin
  Result := 0;
  if ConfigurationFailed then Result := 1;
end;

function InitializeUninstall: Boolean;
var
  ExitCode: Integer;
begin
  Result := Exec(ExpandConstant('{app}\KOL.exe'), '--quit', '', SW_HIDE,
                 ewWaitUntilTerminated, ExitCode);
  if Result then Result := ExitCode = 0;
  if Result then begin
    Result := Exec(ExpandConstant('{app}\KOL.exe'),
      '--configure-startup=disable --data-dir ' + QuotedDataDir(''), '', SW_HIDE,
      ewWaitUntilTerminated, ExitCode);
    if Result then Result := ExitCode = 0;
  end;
  if not Result then
    SuppressibleMsgBox('Impossible de préparer la désinstallation. Fermez KOL et vérifiez que ses préférences sont accessibles.', mbError, MB_OK, IDOK);
end;
