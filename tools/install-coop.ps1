<#
.SYNOPSIS
    JK2 co-op — Windows installer.

.DESCRIPTION
    Stages a co-op install directory next to (never inside) your retail Jedi
    Outcast install, and drops two launcher scripts (jk2coop-host.cmd,
    jk2coop-join.cmd) beside it.

    It never copies or modifies retail files. The retail assets stay where
    Steam put them and are loaded read-only via the engine's fs_cdpath; only
    the co-op engine, renderer, gamecode DLL, and the small Co-op UI overlay
    are copied into the staging directory. -Uninstall removes exactly what the
    installer created (tracked in a manifest), and re-running is idempotent.

    Layout created (default staging dir: %LOCALAPPDATA%\jk2coop):

        jk2coop\
            openjo_sp.x86_64.exe          engine
            rdjosp-vanilla_x86_64.dll     renderer (loaded next to the engine)
            base\
                jospgamex86_64.dll        co-op singleplayer gamecode
                zz-coop-ui.pk3            Co-op menu overlay
        jk2coop-host.cmd                  launcher: host a game
        jk2coop-join.cmd                  launcher: join a game

    At runtime the engine reads:
        fs_basepath = the staging dir   (engine + renderer + base\ co-op files)
        fs_cdpath   = your GameData dir  (retail assets*.pk3, read-only)

.PARAMETER GameData
    Path to your Jedi Outcast "GameData" directory (the one containing
    base\assets0.pk3). If omitted, the installer locates it via the Steam
    registry key and libraryfolders.vdf.

.PARAMETER Binaries
    Directory holding the built Windows binaries (openjo_sp.x86_64.exe,
    rdjosp-vanilla_x86_64.dll, jospgamex86_64.dll). Defaults to a
    jk2coop-windows CI artifact layout under the repo, then the script's own
    directory. See -Help for the search order.

.PARAMETER StagingDir
    Where to install the co-op files. Defaults to %LOCALAPPDATA%\jk2coop.

.PARAMETER Uninstall
    Remove everything this installer created and exit.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 `
        -GameData "D:\Games\Jedi Outcast\GameData" -Binaries .\artifact\RelWithDebInfo

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 -Uninstall
#>
[CmdletBinding()]
param(
    [string]$GameData,
    [string]$Binaries,
    [string]$StagingDir = (Join-Path $env:LOCALAPPDATA 'jk2coop'),
    [switch]$Uninstall
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# --- Constants -------------------------------------------------------------

$EngineExe   = 'openjo_sp.x86_64.exe'
$RendererDll = 'rdjosp-vanilla_x86_64.dll'
$GamecodeDll = 'jospgamex86_64.dll'
$CoopUiPk3   = 'zz-coop-ui.pk3'

$DefaultPort = 29070
$DefaultMap  = 'kejim_post'

$RepoRoot  = Split-Path -Parent $PSScriptRoot          # tools\ -> repo root
$Manifest  = Join-Path $StagingDir '.coop-install-manifest'

# --- Helpers ---------------------------------------------------------------

function Say  ([string]$m) { Write-Host $m }
function Info ([string]$m) { Write-Host "  $m" }
function Die  ([string]$m) { Write-Error $m; exit 1 }

# Record a path we created so -Uninstall can remove exactly it. Idempotent.
function Manifest-Add ([string]$path) {
    $existing = @()
    if (Test-Path -LiteralPath $Manifest) {
        $existing = Get-Content -LiteralPath $Manifest
    }
    if ($existing -notcontains $path) {
        Add-Content -LiteralPath $Manifest -Value $path
    }
}

# Copy a file into place (additive, overwrite our own prior copy) and record it.
function Install-File ([string]$src, [string]$dst) {
    Copy-Item -LiteralPath $src -Destination $dst -Force
    Manifest-Add $dst
}

# --- GameData autodetection ------------------------------------------------

# Parse libraryfolders.vdf and return every library "path" it lists.
function Get-SteamLibraryPaths ([string]$vdf) {
    $paths = @()
    if (-not (Test-Path -LiteralPath $vdf)) { return $paths }
    foreach ($line in Get-Content -LiteralPath $vdf) {
        # Lines look like:   "path"    "D:\\SteamLibrary"
        $m = [regex]::Match($line, '"path"\s*"([^"]+)"')
        if ($m.Success) {
            # VDF escapes backslashes; collapse "\\" back to "\".
            $paths += ($m.Groups[1].Value -replace '\\\\', '\')
        }
    }
    return $paths
}

# Find the GameData dir that contains base\assets0.pk3, or $null.
function Find-GameData {
    # Steam install root from the registry (HKCU first, then HKLM 32-bit view).
    $steamRoots = @()
    foreach ($key in @(
            'HKCU:\Software\Valve\Steam',
            'HKLM:\SOFTWARE\WOW6432Node\Valve\Steam',
            'HKLM:\SOFTWARE\Valve\Steam')) {
        try {
            $p = (Get-ItemProperty -LiteralPath $key -ErrorAction Stop)
            foreach ($prop in @('SteamPath', 'InstallPath')) {
                if ($p.PSObject.Properties.Name -contains $prop -and $p.$prop) {
                    $steamRoots += ($p.$prop -replace '/', '\')
                }
            }
        } catch { }
    }
    # A common default, in case the registry is unhelpful.
    $steamRoots += 'C:\Program Files (x86)\Steam'
    $steamRoots = $steamRoots | Select-Object -Unique

    # Candidate libraries: each Steam root plus everything in its
    # libraryfolders.vdf.
    $libs = @()
    foreach ($root in $steamRoots) {
        if (Test-Path -LiteralPath (Join-Path $root 'steamapps')) { $libs += $root }
        $vdf = Join-Path $root 'steamapps\libraryfolders.vdf'
        $libs += (Get-SteamLibraryPaths $vdf)
    }
    $libs = $libs | Select-Object -Unique

    foreach ($lib in $libs) {
        $gd = Join-Path $lib 'steamapps\common\Jedi Outcast\GameData'
        if (Test-Path -LiteralPath (Join-Path $gd 'base\assets0.pk3')) {
            return $gd
        }
    }
    return $null
}

# --- Binaries discovery ----------------------------------------------------

# Find a directory holding all three built binaries. Search order:
#   1) -Binaries (if given)
#   2) the CI artifact layout under the repo (openjk\build\**, artifact\**)
#   3) the script's own directory (binaries dropped next to the installer)
function Find-Binaries {
    $candidates = @()
    if ($Binaries) { $candidates += $Binaries }
    $candidates += @(
        (Join-Path $RepoRoot 'openjk\build\RelWithDebInfo'),
        (Join-Path $RepoRoot 'openjk\build'),
        (Join-Path $RepoRoot 'artifact\RelWithDebInfo'),
        (Join-Path $RepoRoot 'artifact'),
        $PSScriptRoot
    )
    foreach ($dir in $candidates) {
        if (-not $dir) { continue }
        if ((Test-Path -LiteralPath (Join-Path $dir $EngineExe)) -and
            (Test-Path -LiteralPath (Join-Path $dir $RendererDll)) -and
            (Test-Path -LiteralPath (Join-Path $dir $GamecodeDll))) {
            return (Resolve-Path -LiteralPath $dir).Path
        }
    }
    # Fall back to a recursive search under the repo for the engine, then check
    # its siblings — handles the raw CI artifact tree without a fixed subdir.
    $hit = Get-ChildItem -LiteralPath $RepoRoot -Recurse -Filter $EngineExe -ErrorAction SilentlyContinue |
           Select-Object -First 1
    if ($hit) {
        $dir = $hit.DirectoryName
        if ((Test-Path -LiteralPath (Join-Path $dir $RendererDll)) -and
            (Test-Path -LiteralPath (Join-Path $dir $GamecodeDll))) {
            return $dir
        }
    }
    return $null
}

# --- Uninstall -------------------------------------------------------------

function Invoke-Uninstall {
    Say 'Uninstalling JK2 co-op...'
    if (-not (Test-Path -LiteralPath $Manifest)) {
        Info "no install manifest at $Manifest - nothing tracked to remove."
        return
    }

    $lines = @(Get-Content -LiteralPath $Manifest)
    # Remove files/symlinks first; collect directories to rmdir afterwards.
    $dirs = @()
    foreach ($line in ($lines | Sort-Object -Descending)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        if (Test-Path -LiteralPath $line -PathType Leaf) {
            Remove-Item -LiteralPath $line -Force -ErrorAction SilentlyContinue
            Info "removed $line"
        } elseif (Test-Path -LiteralPath $line -PathType Container) {
            $dirs += $line
        }
    }

    # Manifest lives under the staging dir; remove it before rmdir'ing dirs.
    Remove-Item -LiteralPath $Manifest -Force -ErrorAction SilentlyContinue
    Info 'removed manifest'

    # rmdir tracked directories that are now empty, deepest first. Never force:
    # a dir still holding files we did not create is left in place.
    foreach ($d in ($dirs | Sort-Object -Property Length -Descending)) {
        try {
            if (-not (Get-ChildItem -LiteralPath $d -Force -ErrorAction Stop)) {
                Remove-Item -LiteralPath $d -Force -ErrorAction Stop
                Info "removed dir $d"
            }
        } catch { }
    }

    Say 'Done. Retail files and your Steam install were never touched.'
}

# --- Install ---------------------------------------------------------------

function Invoke-Install {
    Say 'Installing JK2 co-op...'

    # Resolve binaries.
    $binDir = Find-Binaries
    if (-not $binDir) {
        Die @"
could not find the built Windows binaries ($EngineExe, $RendererDll, $GamecodeDll).
       Download the jk2coop-windows CI artifact and pass its folder:
           install-coop.ps1 -Binaries <folder with the 3 files>
"@
    }
    Info "binaries: $binDir"

    # Resolve GameData.
    if (-not $GameData) {
        Say 'Locating your Jedi Outcast GameData...'
        $GameData = Find-GameData
        if (-not $GameData) {
            Die @"
could not find GameData under any Steam library.
       Pass it explicitly:  install-coop.ps1 -GameData "C:\...\Jedi Outcast\GameData"
"@
        }
    }
    if (-not (Test-Path -LiteralPath (Join-Path $GameData 'base\assets0.pk3'))) {
        Die "invalid -GameData: no base\assets0.pk3 under: $GameData"
    }
    $GameData = (Resolve-Path -LiteralPath $GameData).Path
    Info "GameData: $GameData"

    # Stage.
    $baseDir = Join-Path $StagingDir 'base'
    New-Item -ItemType Directory -Force -Path $StagingDir | Out-Null
    New-Item -ItemType Directory -Force -Path $baseDir     | Out-Null
    Manifest-Add $StagingDir
    Manifest-Add $baseDir
    Say "Staging $StagingDir"

    # Engine + renderer at the staging root (the renderer is loaded next to the
    # engine executable).
    Install-File (Join-Path $binDir $EngineExe)   (Join-Path $StagingDir $EngineExe)
    Install-File (Join-Path $binDir $RendererDll) (Join-Path $StagingDir $RendererDll)
    Info "installed engine + renderer"

    # Co-op gamecode DLL into base\ (the SP gamecode loader searches
    # <basepath>\base for jospgamex86_64.dll).
    Install-File (Join-Path $binDir $GamecodeDll) (Join-Path $baseDir $GamecodeDll)
    Info "installed gamecode $GamecodeDll"

    # Co-op UI overlay. Prefer the prebuilt pk3 in the repo; it sorts after the
    # retail assets so its ui\menus.txt wins.
    $coopPk3 = Join-Path $RepoRoot 'assets\coop-ui\zz-coop-ui.pk3'
    if (Test-Path -LiteralPath $coopPk3) {
        Install-File $coopPk3 (Join-Path $baseDir $CoopUiPk3)
        Info "installed co-op UI overlay $CoopUiPk3"
    } else {
        Info "note: $CoopUiPk3 not found in repo - the in-game Co-op menu overlay is skipped."
    }

    # Launchers live beside the staging dir so they are easy to find.
    $hostCmd = Join-Path (Split-Path -Parent $StagingDir) 'jk2coop-host.cmd'
    $joinCmd = Join-Path (Split-Path -Parent $StagingDir) 'jk2coop-join.cmd'
    Say "Installing launchers next to $StagingDir"

    $engine = Join-Path $StagingDir $EngineExe

    # Host launcher: jk2coop-host.cmd [map]
    $hostBody = @"
@echo off
rem jk2coop-host [map] - start a co-op game others can join. Generated by install-coop.ps1.
setlocal
set "MAP=%~1"
if "%MAP%"=="" set "MAP=$DefaultMap"
"$engine" +set fs_basepath "$StagingDir" +set fs_cdpath "$GameData" +set net_enabled 1 +set net_port $DefaultPort +map "%MAP%"
"@
    Set-Content -LiteralPath $hostCmd -Value $hostBody -Encoding ASCII
    Manifest-Add $hostCmd
    Info 'jk2coop-host.cmd'

    # Join launcher: jk2coop-join.cmd <host[:port]>
    $joinBody = @"
@echo off
rem jk2coop-join <host[:port]> - join a co-op game. Generated by install-coop.ps1.
setlocal
if "%~1"=="" (
    echo usage: jk2coop-join ^<host[:port]^>
    exit /b 1
)
set "HOST=%~1"
echo %HOST% | find ":" >nul || set "HOST=%HOST%:$DefaultPort"
"$engine" +set fs_basepath "$StagingDir" +set fs_cdpath "$GameData" +set net_enabled 1 +connect "%HOST%"
"@
    Set-Content -LiteralPath $joinCmd -Value $joinBody -Encoding ASCII
    Manifest-Add $joinCmd
    Info 'jk2coop-join.cmd'

    Say ''
    Say 'Installed. Try:'
    Say "    $hostCmd                 (host on port $DefaultPort)"
    Say "    $joinCmd <host-ip>       (join a game)"
}

# --- Main ------------------------------------------------------------------

if ($Uninstall) {
    Invoke-Uninstall
} else {
    Invoke-Install
}
