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

.PARAMETER WithWidescreen
    Enable the widescreen / QHD / ultrawide / 4K video-menu mod (adds the modern
    resolutions to SETUP > VIDEO > Video Mode). Built natively in PowerShell from
    your own retail menu files; no retail data is modified.

.PARAMETER WithTextures
    Generate the AI material-texture pak. This needs an AMD ROCm GPU container,
    which is a Linux-only setup; on Windows the installer prints the command to
    run on a suitable Linux machine rather than running it.

.PARAMETER WithUpscale
    Build the Real-ESRGAN hi-res texture override. Also a Linux GPU-only mod;
    offered here as a printed command.

.PARAMETER Combat
    Combat feel: 'modern' (default; free aim, fixed screen-center crosshair,
    FOV-independent sensitivity, faster bolts) or 'classic' (legacy auto-aim,
    dynamic muzzle-traced crosshair, FOV-linked sensitivity). Written to
    base\autoexec_sp.cfg so it overrides a stale openjo_sp.cfg on disk.

.PARAMETER SkipCutscenes
    Auto-skip scripted map-intro cutscenes (off by default).

.PARAMETER NoSkipCutscenes
    Never auto-skip cutscenes (suppress the prompt).

.PARAMETER Sensitivity
    Base mouse sensitivity written in modern mode (default 0.5; the JK2 engine
    default is 5, which is fast on a modern high-DPI mouse). Ignored with
    -Combat classic.

.PARAMETER Render
    Render fidelity: 'high' (default; sharper textures, better filtering, and the
    software-overbright lighting fix so world/models keep their punch on
    windowed/borderless setups) or 'classic' (retail engine defaults). Written to
    base\autoexec_render.cfg. See docs/render-fidelity.md.

.PARAMETER All
    Enable every optional mod above.

.PARAMETER NoOptional
    Skip all optional-mod prompts (core install only).

.PARAMETER Yes
    Assume "yes" to any optional-mod prompt that would otherwise be shown.

.PARAMETER Uninstall
    Remove everything this installer created and exit.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File tools\install-coop.ps1 -All

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
    [switch]$WithWidescreen,
    [switch]$WithTextures,
    [switch]$WithUpscale,
    [ValidateSet('modern','classic')]
    [string]$Combat = 'modern',
    [switch]$SkipCutscenes,
    [switch]$NoSkipCutscenes,
    [double]$Sensitivity = 0.5,
    [ValidateSet('high','classic')]
    [string]$Render = 'high',
    [switch]$All,
    [switch]$NoOptional,
    [switch]$Yes,
    [switch]$Uninstall
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# --- Constants -------------------------------------------------------------

$EngineExe   = 'openjo_sp.x86_64.exe'
$RendererDll = 'rdjosp-vanilla_x86_64.dll'
$GamecodeDll = 'jospgamex86_64.dll'
$CoopUiPk3   = 'zz-coop-ui.pk3'
$Sdl2Dll     = 'SDL2.dll'

# Visual C++ 2015-2022 x64 redistributable. The MSVC-built engine links the
# dynamic CRT (VCRUNTIME140.dll / MSVCP140.dll); a fresh Windows install does
# not ship it, so the engine would fail to start without it.
$VcRedistUrl   = 'https://aka.ms/vs/17/release/vc_redist.x64.exe'
$VcRedistProbe = 'MSVCP140.dll'

$DefaultPort = 29070
$DefaultMap  = 'kejim_post'

$RepoRoot  = Split-Path -Parent $PSScriptRoot          # tools\ -> repo root
$Manifest  = Join-Path $StagingDir '.coop-install-manifest'

# The stock SP video-mode resolution list ends at 2048x1536 (mode 10). The
# widescreen mod appends the Track-G engine modes (13-21). Match exactly, and
# skip if the line is not in the stock form (already patched / different edition).
$WsTail = '@MENUS1_2048_X_1536 10 }'
$WsAdd  = '@MENUS1_2048_X_1536 10  "1280 X 720 (16:9)" 13  "1600 X 900 (16:9)" 14  "1920 X 1080 (16:9)" 15  "2560 X 1080 (21:9)" 16  "2560 X 1440 QHD" 17  "3440 X 1440 (21:9)" 18  "3840 X 1600 (24:10)" 19  "3840 X 2160 4K" 20  "5120 X 1440 (32:9)" 21 }'

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

# Ensure the Visual C++ 2015-2022 x64 redistributable is present. The engine is
# built with MSVC against the dynamic CRT, so without it the engine fails to
# start with "VCRUNTIME140.dll / MSVCP140.dll was not found". This is a system
# component (installed once, machine-wide) and is NOT tracked in the manifest —
# -Uninstall leaves it in place, since other software may rely on it.
function Install-VcRedist {
    # Not applicable off Windows (e.g. running the script's tests under
    # PowerShell on Linux). $IsWindows is $null on Windows PowerShell 5.1,
    # where the script normally runs, so treat null as Windows.
    if ($IsWindows -eq $false) { return }
    $winRoot = $env:SystemRoot
    if (-not $winRoot) { $winRoot = 'C:\Windows' }
    if ([System.IO.File]::Exists((Join-Path (Join-Path $winRoot 'System32') $VcRedistProbe))) {
        Info 'Visual C++ redistributable already present'
        return
    }
    Say 'Installing the Visual C++ redistributable (required by the engine)...'
    $tmp = Join-Path $env:TEMP 'vc_redist.x64.exe'
    try {
        Invoke-WebRequest -Uri $VcRedistUrl -OutFile $tmp -UseBasicParsing
        $p = Start-Process -FilePath $tmp -ArgumentList '/install', '/quiet', '/norestart' -Wait -PassThru
        Remove-Item -LiteralPath $tmp -Force -ErrorAction SilentlyContinue
        if ($p.ExitCode -eq 0 -or $p.ExitCode -eq 3010) {
            Info 'Visual C++ redistributable installed'
            if ($p.ExitCode -eq 3010) {
                Info 'note: a reboot may be required before the engine will start.'
            }
        } else {
            Info "warning: Visual C++ redistributable installer exited with code $($p.ExitCode); install it manually if the engine will not start."
        }
    } catch {
        Info "warning: could not install the Visual C++ redistributable automatically ($($_.Exception.Message))."
        Info "         Download it from $VcRedistUrl and install it if the engine will not start."
    }
}

# --- Optional mods ---------------------------------------------------------

# True if we can prompt the user (interactive host with a real console).
function Test-Interactive {
    try { return [Environment]::UserInteractive -and -not [Console]::IsInputRedirected }
    catch { return $false }
}

# Ask a yes/no question (default No). -Yes auto-confirms a prompt that would
# otherwise be shown; it does NOT turn an un-prompted question into a yes.
function Confirm-Mod ([string]$prompt) {
    if (-not (Test-Interactive)) { return $false }
    if ($Yes) { Say "  $prompt [y/N] y (-Yes)"; return $true }
    $reply = Read-Host "  $prompt [y/N]"
    return ($reply -match '^(y|yes)$')
}

# Resolve a mod decision: an explicit flag ($true) forces yes; -All forces yes;
# -NoOptional forces no; otherwise prompt (interactive) or default no.
function Resolve-Mod ([bool]$flag, [string]$prompt) {
    if ($NoOptional) { return $false }
    if ($flag -or $All) { return $true }
    return (Confirm-Mod $prompt)
}

# Build the widescreen video-menu override pak natively (no bash dependency).
# Reads the two SP menu files from the user's own retail assets1.pk3, appends the
# widescreen resolution entries to the single Video Mode list line, and writes a
# zz-widescreen-menu.pk3. Returns $true on success. Retail data is untouched.
function Build-WidescreenPak ([string]$gameData, [string]$outPk3) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem -ErrorAction SilentlyContinue

    # Locate the retail pak that carries the SP video menu (has ui_r_mode).
    $srcPak = $null
    foreach ($p in (Get-ChildItem -LiteralPath (Join-Path $gameData 'base') -Filter 'assets*.pk3' -ErrorAction SilentlyContinue | Sort-Object Name)) {
        try {
            $z = [System.IO.Compression.ZipFile]::OpenRead($p.FullName)
            try {
                $e = $z.Entries | Where-Object { $_.FullName -eq 'ui/ingamesetup.menu' } | Select-Object -First 1
                if ($e) {
                    $sr = New-Object System.IO.StreamReader($e.Open())
                    $txt = $sr.ReadToEnd(); $sr.Close()
                    if ($txt -match 'ui_r_mode') { $srcPak = $p.FullName }
                }
            } finally { $z.Dispose() }
        } catch { }
        if ($srcPak) { break }
    }
    if (-not $srcPak) { Info 'widescreen: no assets*.pk3 with the SP video menu found; skipped.'; return $false }

    $work = Join-Path ([System.IO.Path]::GetTempPath()) ("jk2-ws-" + [System.IO.Path]::GetRandomFileName())
    $uiDir = Join-Path $work 'ui'
    New-Item -ItemType Directory -Force -Path $uiDir | Out-Null
    try {
        $patched = 0
        $zip = [System.IO.Compression.ZipFile]::OpenRead($srcPak)
        try {
            foreach ($name in @('ingamesetup.menu', 'setup.menu')) {
                $entry = $zip.Entries | Where-Object { $_.FullName -eq "ui/$name" } | Select-Object -First 1
                if (-not $entry) { continue }
                # Read as latin-1 (ISO-8859) bytes->string to preserve the file exactly.
                $ms = New-Object System.IO.MemoryStream
                $es = $entry.Open(); $es.CopyTo($ms); $es.Close()
                $enc = [System.Text.Encoding]::GetEncoding('iso-8859-1')
                $content = $enc.GetString($ms.ToArray())
                $count = ([regex]::Matches($content, [regex]::Escape($WsTail))).Count
                if ($count -ne 1) {
                    Info "widescreen: $name resolution list not in the expected stock form; skipped."
                    continue
                }
                $content = $content.Replace($WsTail, $WsAdd)
                [System.IO.File]::WriteAllBytes((Join-Path $uiDir $name), $enc.GetBytes($content))
                $patched++
            }
        } finally { $zip.Dispose() }

        if ($patched -lt 1) { Info 'widescreen: no menu files could be patched.'; return $false }

        if (Test-Path -LiteralPath $outPk3) { Remove-Item -LiteralPath $outPk3 -Force }
        # Zip the ui\ tree into the pak.
        [System.IO.Compression.ZipFile]::CreateFromDirectory($work, $outPk3)
        return $true
    } finally {
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Build the sensitivity-slider override pak natively. Reads the two SP control
# menus from the user's own retail assets*.pk3, rewrites only the one sensitivity
# cvarfloat line (retail "5 2 30" -> "0.5 0.1 2"), and writes zz-sensitivity-menu.pk3
# so the CONTROLS mouse slider can reach the lower modern values. Returns $true on
# success. Retail data is untouched.
function Build-SensitivityPak ([string]$baseDir, [string]$outPk3) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem -ErrorAction SilentlyContinue
    $enc  = [System.Text.Encoding]::GetEncoding('iso-8859-1')
    $stock = "cvarfloat`t`t`t`"sensitivity`" 5 2 30"
    $new   = "cvarfloat`t`t`t`"sensitivity`" 0.5 0.1 2"

    # Read-only-ish value readout, same group as the slider so it shows/hides with
    # the MOUSE/JOYSTICK page. ITEM_TYPE_EDITFIELD repaints the cvar every frame, so
    # it live-updates as the slider drags; clicking it lets you type an exact value.
    $t = "`t"
    $readout =
        "`r`n" +
        "$t${t}itemDef `r`n" +
        "$t$t{`r`n" +
        "$t$t${t}name${t}${t}${t}sensitivityvalue`r`n" +
        "$t$t${t}group${t}${t}${t}joycontrols`r`n" +
        "$t$t${t}type${t}${t}${t}ITEM_TYPE_EDITFIELD`r`n" +
        "$t$t${t}style${t}${t}${t}WINDOW_STYLE_EMPTY`r`n" +
        "$t$t${t}cvar${t}${t}${t}`"sensitivity`"`r`n" +
        "$t$t${t}maxChars${t}${t}8`r`n" +
        "$t$t${t}rect${t}${t}${t}594 211 46 20`r`n" +
        "$t$t${t}textalign${t}${t}ITEM_ALIGN_LEFT`r`n" +
        "$t$t${t}textalignx${t}${t}0`r`n" +
        "$t$t${t}textaligny${t}${t}-2`r`n" +
        "$t$t${t}font${t}${t}${t}2`r`n" +
        "$t$t${t}textscale${t}${t}0.8`r`n" +
        "$t$t${t}forecolor${t}${t}1 1 1 1`r`n" +
        "$t$t${t}backcolor${t}${t}0 0 0 0`r`n" +
        "$t$t${t}visible${t}${t}${t}0 `r`n" +
        "$t$t}`r`n"
    $sliderClose = "`r`n$t$t}`r`n"

    $work = Join-Path ([System.IO.Path]::GetTempPath()) ("jk2-sens-" + [System.IO.Path]::GetRandomFileName())
    $uiDir = Join-Path $work 'ui'
    New-Item -ItemType Directory -Force -Path $uiDir | Out-Null
    try {
        $patched = 0
        foreach ($name in @('controls.menu', 'ingamecontrols.menu')) {
            # Find the first retail pak carrying this menu (case-insensitive entry).
            foreach ($p in (Get-ChildItem -LiteralPath $baseDir -Filter 'assets*.pk3' -ErrorAction SilentlyContinue | Sort-Object Name)) {
                $zip = [System.IO.Compression.ZipFile]::OpenRead($p.FullName)
                try {
                    $entry = $zip.Entries | Where-Object { $_.FullName -ieq "ui/$name" } | Select-Object -First 1
                    if (-not $entry) { continue }
                    $ms = New-Object System.IO.MemoryStream
                    $es = $entry.Open(); $es.CopyTo($ms); $es.Close()
                    $content = $enc.GetString($ms.ToArray())
                    if (-not $content.Contains($stock)) { continue }
                    $content = $content.Replace($stock, $new)
                    # Inject the readout right after the slider's itemDef close (the
                    # first "\r\n\t\t}\r\n" after the rescaled cvarfloat line).
                    $ai = $content.IndexOf($new)
                    $ci = $content.IndexOf($sliderClose, $ai)
                    if ($ci -ge 0) {
                        $insAt = $ci + $sliderClose.Length
                        $content = $content.Substring(0, $insAt) + $readout + $content.Substring($insAt)
                    }
                    # Emit at the lowercase path the menu loader references.
                    [System.IO.File]::WriteAllBytes((Join-Path $uiDir $name), $enc.GetBytes($content))
                    $patched++
                    break
                } finally { $zip.Dispose() }
            }
        }
        if ($patched -lt 1) { Info 'sensitivity: no control menu with the stock slider found; skipped.'; return $false }
        if (Test-Path -LiteralPath $outPk3) { Remove-Item -LiteralPath $outPk3 -Force }
        [System.IO.Compression.ZipFile]::CreateFromDirectory($work, $outPk3)
        return $true
    } finally {
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Write autoexec_render.cfg with the render-fidelity preset (see install-coop.sh
# for the full rationale). Exec'd from autoexec_sp.cfg. Manifest-tracked.
function Write-RenderConfig ([string]$baseDir) {
    $cfg = Join-Path $baseDir 'autoexec_render.cfg'
    if ($Render -eq 'high') {
        $lines = @(
            "// Written by install-coop.ps1 - render fidelity preset ($Render)."
            '// Delete this file (or re-run with -Render classic) to revert.'
            'seta r_overBrightBitsSoftware "1"'
            'seta r_overBrightBits "1"'
            'seta r_mapOverBrightBits "2"'
            # Neutral gamma: a high saved r_gamma washes out the overbright.
            'seta r_gamma "1.0"'
            'seta r_picmip "0"'
            'seta r_ext_compress_textures "0"'
            'seta r_texturebits "32"'
            'seta r_ext_texture_filter_anisotropic "16"'
            'seta r_textureMode "GL_LINEAR_MIPMAP_LINEAR"'
            # Edge anti-aliasing (MSAA); latched, falls back if unsupported.
            'seta r_ext_multisample "8"'
            'seta r_subdivisions "1"'
            'seta r_lodbias "-2"'
            'seta r_lodscale "20"'
        )
    } else {
        $lines = @(
            "// Written by install-coop.ps1 - render fidelity preset ($Render)."
            '// Delete this file (or re-run with -Render classic) to revert.'
            'seta r_overBrightBitsSoftware "0"'
            'seta r_overBrightBits "0"'
            'seta r_mapOverBrightBits "0"'
            'seta r_gamma "1.0"'
            'seta r_picmip "0"'
            'seta r_ext_compress_textures "1"'
            'seta r_texturebits "0"'
            'seta r_ext_texture_filter_anisotropic "16"'
            'seta r_textureMode "GL_LINEAR_MIPMAP_LINEAR"'
            'seta r_ext_multisample "0"'
            'seta r_subdivisions "4"'
            'seta r_lodbias "0"'
            'seta r_lodscale "10"'
        )
    }
    Set-Content -LiteralPath $cfg -Value $lines -Encoding ASCII
    Manifest-Add $cfg
    Info "wrote autoexec_render.cfg: render=$Render"
}

# Write autoexec_sp.cfg with the modern-combat cvars (or classic), plus optional
# cutscene auto-skip. The engine execs autoexec_sp.cfg on startup, after
# openjo_sp.cfg, so these win over a stale config on disk. Manifest-tracked.
function Write-CombatConfig ([string]$baseDir) {
    if ($Combat -eq 'classic') {
        $aim = 1; $xhair = 1; $sens = 1
        $desc = 'classic (legacy auto-aim, dynamic crosshair, FOV-linked sensitivity)'
    } else {
        $aim = 0; $xhair = 0; $sens = 0
        $desc = 'modern (free aim, fixed crosshair, FOV-independent sensitivity)'
    }

    $skip = 0
    if ($SkipCutscenes) {
        $skip = 1
    } elseif (-not $NoSkipCutscenes) {
        if (Resolve-Mod $false 'Auto-skip scripted map-intro cutscenes?') { $skip = 1 }
    }

    $cfg = Join-Path $baseDir 'autoexec_sp.cfg'
    $lines = @(
        '// Written by install-coop.ps1 - modern combat feel.'
        '// Delete this file (or re-run with -Combat classic) to change it.'
        "seta g_saberAutoAim `"$aim`""
        "seta cg_dynamicCrosshair `"$xhair`""
        "seta cg_fovSensitivityScale `"$sens`""
        "seta g_skipIntroCinematics `"$skip`""
    )
    if ($Combat -ne 'classic') {
        # Invariant culture so a comma decimal locale can't write "0,5".
        $sensStr = $Sensitivity.ToString([System.Globalization.CultureInfo]::InvariantCulture)
        $lines += "seta sensitivity `"$sensStr`""
    }
    # Chain the render-fidelity preset (only autoexec_sp.cfg is auto-exec'd).
    $lines += 'exec autoexec_render.cfg'
    Set-Content -LiteralPath $cfg -Value $lines -Encoding ASCII
    Manifest-Add $cfg
    Info "wrote autoexec_sp.cfg: combat=$desc, cutscene-skip=$skip"

    Write-RenderConfig $baseDir

    # In modern mode, rescale the CONTROLS mouse-sensitivity slider (retail min 2)
    # so the UI can reach the lower modern values.
    if ($Combat -ne 'classic') {
        $smPak = Join-Path $baseDir 'zz-sensitivity-menu.pk3'
        try {
            if (Build-SensitivityPak $baseDir $smPak) {
                Manifest-Add $smPak
                Info 'installed zz-sensitivity-menu.pk3 (CONTROLS mouse slider: 0.1-2)'
            }
        } catch {
            Info "sensitivity-menu build failed ($($_.Exception.Message))."
        }
    }
}

# Run the optional-mod stage after the core install.
function Invoke-OptionalMods ([string]$gameData, [string]$baseDir) {
    $any = $false

    # Widescreen / QHD / ultrawide (native; no GPU needed).
    if (Resolve-Mod ([bool]$WithWidescreen) 'Add widescreen / QHD / ultrawide / 4K resolutions to the video menu?') {
        $any = $true
        $wsPak = Join-Path $baseDir 'zz-widescreen-menu.pk3'
        Say 'Enabling widescreen video-menu modes...'
        try {
            if (Build-WidescreenPak $gameData $wsPak) {
                Manifest-Add $wsPak
                Info 'installed zz-widescreen-menu.pk3 (SETUP > VIDEO > Video Mode)'
            } else {
                Info 'widescreen mod was not applied.'
            }
        } catch {
            Info "widescreen build failed ($($_.Exception.Message))."
        }
    }

    # AI-generated textures (Linux GPU-only).
    if (Resolve-Mod ([bool]$WithTextures) 'Generate original AI material textures? (needs a Linux GPU + container)') {
        $any = $true
        Info 'AI texture generation needs an AMD ROCm GPU container, which is a Linux-only setup.'
        Info 'run it on a suitable Linux machine, then copy the resulting pak into base\:'
        Info '    tools/generate-textures.sh --out zzz-generated-textures.pk3'
        Info '(see docs/asset-generation.md)'
    }

    # Real-ESRGAN upscale (Linux GPU-only).
    if (Resolve-Mod ([bool]$WithUpscale) 'Build a Real-ESRGAN hi-res texture override? (needs a Linux GPU + container)') {
        $any = $true
        Info 'Real-ESRGAN upscaling needs an AMD ROCm GPU container, which is a Linux-only setup.'
        Info 'run it on a suitable Linux machine, then copy the resulting pak into base\:'
        Info "    tools/upscale-textures.sh --assets `"$gameData\base`" --out zzz-hires-textures.pk3"
        Info '(see docs/hires-textures.md)'
    }

    # Modern combat feel + optional cutscene skip (always writes autoexec_sp.cfg).
    Write-CombatConfig $baseDir
    $any = $true

    if (-not $any) { Info 'no optional mods selected.' }
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

    # SDL2.dll is a runtime dependency loaded next to the engine. The
    # jk2coop-windows artifact ships it; copy it if present.
    $sdlSrc = Join-Path $binDir $Sdl2Dll
    if (Test-Path -LiteralPath $sdlSrc) {
        Install-File $sdlSrc (Join-Path $StagingDir $Sdl2Dll)
        Info "installed $Sdl2Dll"
    } else {
        Info "note: $Sdl2Dll not found next to the binaries - the engine needs it to start."
    }

    # Ensure the Visual C++ redistributable the MSVC engine links against.
    Install-VcRedist

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

    # Optional game-file mods (widescreen; textures/upscale are Linux GPU-only).
    Say ''
    Say 'Optional mods:'
    Invoke-OptionalMods $GameData $baseDir

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
