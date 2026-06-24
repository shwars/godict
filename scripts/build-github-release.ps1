[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$')]
    [string]$Version,

    [Parameter(Mandatory)]
    [ValidateSet('windows-amd64', 'linux-amd64', 'darwin-arm64', 'windows-arm64')]
    [string]$Target,

    [switch]$WindowsArm64Docker
)

$ErrorActionPreference = 'Stop'
$root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$releases = Join-Path $root 'releases'
$stageName = "godict_${Version}_$($Target.Replace('-', '_'))"
$stage = Join-Path $releases $stageName
$definitions = @{
    'windows-amd64' = @{ OS = 'windows'; Arch = 'amd64'; Executable = 'godict.exe' }
    'linux-amd64'   = @{ OS = 'linux'; Arch = 'amd64'; Executable = 'godict' }
    'darwin-arm64'  = @{ OS = 'darwin'; Arch = 'arm64'; Executable = 'godict' }
    'windows-arm64' = @{ OS = 'windows'; Arch = 'arm64'; Executable = 'godict.exe' }
}
$targetDefinition = $definitions[$Target]

if ($Target -eq 'windows-arm64' -and -not $WindowsArm64Docker) {
    throw 'windows-arm64 requires -WindowsArm64Docker.'
}
if ($Target -ne 'windows-arm64' -and $WindowsArm64Docker) {
    throw '-WindowsArm64Docker is valid only for windows-arm64.'
}

New-Item -ItemType Directory -Force -Path $releases | Out-Null
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $stage
New-Item -ItemType Directory -Force -Path $stage | Out-Null
Push-Location $root
try {
    $destination = Join-Path $stage $targetDefinition.Executable
    if ($WindowsArm64Docker) {
        & go run github.com/fyne-io/fyne-cross@v1.6.2 windows -dir $root '-arch=arm64' -app-id net.godict.desktop -app-version $Version -name godict
        if ($LASTEXITCODE -ne 0) { throw 'Fyne cross-build failed for windows/arm64.' }
        $binary = Get-ChildItem -Path (Join-Path $root 'fyne-cross') -Recurse -File | Where-Object Name -eq 'godict.exe' | Select-Object -Last 1
        if (-not $binary) { throw 'Fyne cross-build did not produce godict.exe.' }
        Copy-Item $binary.FullName $destination
    } else {
        $hostOS = (go env GOOS).Trim()
        $hostArch = (go env GOARCH).Trim()
        if ($hostOS -ne $targetDefinition.OS -or $hostArch -ne $targetDefinition.Arch) {
            throw "Native target $Target requires $($targetDefinition.OS)/$($targetDefinition.Arch), got $hostOS/$hostArch."
        }
        if ($Target -eq 'windows-amd64') {
            # `go build` creates a generic Windows executable icon. Fyne's
            # packager embeds Icon.png as the PE application resource and also
            # selects the Windows GUI subsystem, so launching GoDict does not
            # leave a console attached to the process.
            Push-Location $stage
            try {
                & go run fyne.io/fyne/v2/cmd/fyne@v2.6.3 package `
                    -os windows `
                    -src $root `
                    -executable $destination `
                    -name godict `
                    -icon (Join-Path $root 'Icon.png') `
                    -appID net.godict.desktop `
                    -release
                if ($LASTEXITCODE -ne 0) { throw 'Fyne package failed for windows/amd64.' }
            }
            finally { Pop-Location }
            if (-not (Test-Path $destination)) {
                throw 'Fyne package did not produce godict.exe.'
            }
        } else {
            & go build -trimpath '-ldflags=-s -w' -o $destination .
            if ($LASTEXITCODE -ne 0) { throw "Native build failed for $Target." }
        }
    }
    foreach ($config in 'godict.config', 'godict.ru.config', 'godict.en.config') {
        Copy-Item (Join-Path $root $config) (Join-Path $stage $config)
    }
    Compress-Archive -Path (Join-Path $stage '*') -DestinationPath (Join-Path $releases "$stageName.zip") -Force
}
finally { Pop-Location; Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $stage }
