# Native Windows smoke test. Does not register a browser, launch a URL, or use real profiles.
[CmdletBinding()]
param([string] $App = "dist/LinkSwitch-windows-amd64.exe")

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
$exe = (Resolve-Path -LiteralPath $App).Path
if (-not ("LinkSwitchSmoke.NativeWindow" -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
namespace LinkSwitchSmoke {
    public static class NativeWindow {
        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        public static extern int GetClassName(IntPtr window, StringBuilder text, int maxCount);
    }
}
'@
}
$originalLocalAppData = $env:LOCALAPPDATA
$testHome = Join-Path ([IO.Path]::GetTempPath()) ("linkswitch-smoke-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $testHome | Out-Null
$env:LOCALAPPDATA = $testHome

function Test-Window([string[]] $LaunchArguments) {
    $options = @{ FilePath = $exe; PassThru = $true }
    if ($LaunchArguments.Count -gt 0) { $options.ArgumentList = $LaunchArguments }
    $process = Start-Process @options
    try {
        $deadline = [DateTime]::UtcNow.AddSeconds(12)
        do {
            Start-Sleep -Milliseconds 100
            $process.Refresh()
            if ($process.HasExited) { throw "LinkSwitch closed before showing its window." }
            if ($process.MainWindowTitle -eq "LinkSwitch") { break }
        } while ([DateTime]::UtcNow -lt $deadline)
        if ($process.MainWindowHandle -eq [IntPtr]::Zero -or $process.MainWindowTitle -ne "LinkSwitch") {
            throw "LinkSwitch window not found."
        }
        $className = [Text.StringBuilder]::new(128)
        [void][LinkSwitchSmoke.NativeWindow]::GetClassName($process.MainWindowHandle, $className, $className.Capacity)
        if ($className.ToString() -ne "LinkSwitch.Main") { throw "Unexpected LinkSwitch window: $className" }
        if (-not $process.CloseMainWindow() -or -not $process.WaitForExit(4000)) {
            throw "LinkSwitch did not close normally."
        }
        if ($process.ExitCode -ne 0) { throw "LinkSwitch exited with code $($process.ExitCode)." }
    } finally {
        if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force }
        $process.Dispose()
    }
}

try {
    Test-Window @()
    Test-Window @("--url", "https://example.invalid/")
} finally {
    $env:LOCALAPPDATA = $originalLocalAppData
    Remove-Item -LiteralPath $testHome -Recurse -Force
}
