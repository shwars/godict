[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$')]
    [string]$Version,

    # Select one or more exact OS/architecture combinations. Omit for all targets.
    [ValidateSet('windows-amd64', 'windows-arm64', 'linux-amd64', 'darwin-amd64', 'darwin-arm64')]
    [string[]]$Targets = @('windows-amd64', 'windows-arm64', 'linux-amd64', 'darwin-arm64')
)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$releases = Join-Path $root 'releases'
$intermediate = Join-Path $root 'fyne-cross'

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw 'Docker is required for cross-platform builds. Install and start Docker first.'
}

New-Item -ItemType Directory -Force -Path $releases | Out-Null
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $intermediate

$targetDefinitions = @{
    'windows-amd64' = @{ OS = 'windows'; Arch = 'amd64'; Executable = 'godict.exe' }
    'windows-arm64' = @{ OS = 'windows'; Arch = 'arm64'; Executable = 'godict.exe' }
    'linux-amd64'   = @{ OS = 'linux';   Arch = 'amd64'; Executable = 'godict' }
    'darwin-amd64'  = @{ OS = 'darwin';  Arch = 'amd64'; Executable = 'godict' }
    'darwin-arm64'  = @{ OS = 'darwin';  Arch = 'arm64'; Executable = 'godict' }
}
$selectedTargets = foreach ($targetName in $Targets) { $targetDefinitions[$targetName] }

Push-Location $root
try {
    foreach ($target in $selectedTargets) {
        Write-Host "Building $($target.OS)/$($target.Arch)..."
        # The maintained launcher selects the platform-specific fyne-cross-images
        # container (the former fyneio/fyne-cross:latest image no longer exists).
        & go run github.com/fyne-io/fyne-cross@v1.6.2 $target.OS `
            -dir $root `
            "-arch=$($target.Arch)" `
            -app-id net.godict.desktop `
            -app-version $Version `
            -name godict
        if ($LASTEXITCODE -ne 0) { throw "Fyne cross-build failed for $($target.OS)/$($target.Arch)." }

        $binary = Get-ChildItem -Path $intermediate -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -eq $target.Executable } |
            Select-Object -Last 1
        if (-not $binary) { throw "Fyne cross-build did not produce $($target.Executable) for $($target.OS)/$($target.Arch)." }

        $name = "godict_${Version}_$($target.OS)_$($target.Arch)"
        $stage = Join-Path $releases $name
        New-Item -ItemType Directory -Force -Path $stage | Out-Null
        Copy-Item -LiteralPath $binary.FullName -Destination (Join-Path $stage $target.Executable)
        Copy-Item -LiteralPath (Join-Path $root 'godict.config') -Destination (Join-Path $stage 'godict.config')
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath (Join-Path $releases "$name.zip") -Force
        Remove-Item -Recurse -Force $stage
    }
}
finally {
    Pop-Location
}

Write-Host "Release archives created in $releases"
