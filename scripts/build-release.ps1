param(
    [ValidateRange(0, 65535)]
    [int]$BuildNumber = 0,
    [ValidateSet('windows/amd64', 'linux/amd64', 'linux/arm64', 'darwin/universal')]
    [string]$Platform = 'windows/amd64'
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $projectRoot 'mygo.json'
$originalConfig = [IO.File]::ReadAllBytes($configPath)
$config = [Text.Encoding]::UTF8.GetString($originalConfig) | ConvertFrom-Json
if ($config.version -notmatch '^(\d+)\.(\d+)\.(\d+)$') {
    throw 'mygo.json version must use major.minor.patch.'
}

# Workflow runs (including PR checks) each reserve a patch number. Reruns
# reuse their number, and local builds use the configured version by default.
$patch = [int]$Matches[3] + $BuildNumber
if ($patch -gt 65535) {
    throw 'Release patch exceeds the Windows version limit; start a new minor version.'
}
$version = '{0}.{1}.{2}' -f $Matches[1], $Matches[2], $patch
$config.version = $version
$tag = "v$version"
$target = $Platform.Replace('/', '-')
$relativeDirectory = "build/releases/$tag/$target"
$releaseDirectory = Join-Path $projectRoot $relativeDirectory
$buildDirectory = Join-Path $projectRoot 'build/release-build'
$previousCGO = $env:CGO_ENABLED

Push-Location $projectRoot
try {
    $env:CGO_ENABLED = '0'
    [IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
    go tool mygo build -platform $Platform -o $buildDirectory
    if ($LASTEXITCODE -ne 0) { throw "MyGo build failed with exit code $LASTEXITCODE." }

    $platformDirectory = Join-Path $buildDirectory $target
    # MyGo 0.7 names installers and archives using the configured slug.
    $files = switch ($Platform) {
        'windows/amd64' {
            @{ Source = "$($config.name).exe"; Name = "$($config.name)-v$version.exe" }
            @{ Source = "$($config.slug)-setup-$version-amd64.exe"; Name = "$($config.name)-setup-v$version.exe" }
        }
        'darwin/universal' {
            @{ Source = "$($config.slug)-$version.dmg"; Name = "$($config.name)-v$version-$target.dmg" }
        }
        default {
            $architecture = $Platform.Split('/')[1]
            @{ Source = "$($config.slug)-$version-$target.tar.gz"; Name = "$($config.name)-v$version-$target.tar.gz" }
            @{ Source = "$($config.slug)_${version}_$architecture.deb"; Name = "$($config.name)-v$version-$target.deb" }
        }
    }
    foreach ($file in $files) {
        if (!(Test-Path -LiteralPath (Join-Path $platformDirectory $file.Source) -PathType Leaf)) {
            throw "Required release file is missing: $($file.Source)"
        }
    }

    New-Item -ItemType Directory -Path $releaseDirectory -Force | Out-Null
    $checksums = foreach ($file in $files) {
        $destination = Join-Path $releaseDirectory $file.Name
        Copy-Item -LiteralPath (Join-Path $platformDirectory $file.Source) -Destination $destination -Force
        $hash = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($file.Name)"
    }
    $checksumName = "$($config.name)-v$version-$target.sha256"
    [IO.File]::WriteAllLines((Join-Path $releaseDirectory $checksumName), [string[]]$checksums, [Text.UTF8Encoding]::new($false))

    if ($env:GITHUB_OUTPUT) {
        @("version=$version", "tag=$tag", "directory=$relativeDirectory", "artifact=$($config.name)-v$version-$target") |
            Add-Content -LiteralPath $env:GITHUB_OUTPUT -Encoding utf8
    }
    Write-Host "Release files: $releaseDirectory"
}
finally {
    [IO.File]::WriteAllBytes($configPath, $originalConfig)
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
