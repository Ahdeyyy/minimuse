param(
    [ValidateRange(0, 65535)]
    [int]$BuildNumber = 0
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
$relativeDirectory = "build/releases/$tag"
$releaseDirectory = Join-Path $projectRoot $relativeDirectory
$buildDirectory = Join-Path $projectRoot 'build/release-build'
$previousCGO = $env:CGO_ENABLED

Push-Location $projectRoot
try {
    $env:CGO_ENABLED = '0'
    [IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
    go tool mygo build -platform windows/amd64 -o $buildDirectory
    if ($LASTEXITCODE -ne 0) { throw "MyGo build failed with exit code $LASTEXITCODE." }

    $platformDirectory = Join-Path $buildDirectory 'windows-amd64'
    $executable = Join-Path $platformDirectory "$($config.name).exe"
    # MyGo 0.7 names Windows installers using the configured slug.
    $installer = Join-Path $platformDirectory "$($config.slug)-setup-$version-amd64.exe"
    if (!(Test-Path -LiteralPath $executable -PathType Leaf) -or
        !(Test-Path -LiteralPath $installer -PathType Leaf)) {
        throw 'The Windows executable and NSIS installer are both required.'
    }

    New-Item -ItemType Directory -Path $releaseDirectory -Force | Out-Null
    $portableName = "$($config.name)-v$version.exe"
    $installerName = "$($config.name)-setup-v$version.exe"
    Copy-Item -LiteralPath $executable -Destination (Join-Path $releaseDirectory $portableName) -Force
    Copy-Item -LiteralPath $installer -Destination (Join-Path $releaseDirectory $installerName) -Force
    $checksums = foreach ($name in @($portableName, $installerName)) {
        $hash = (Get-FileHash -LiteralPath (Join-Path $releaseDirectory $name) -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $name"
    }
    [IO.File]::WriteAllLines((Join-Path $releaseDirectory "$($config.name)-v$version.sha256"), [string[]]$checksums, [Text.UTF8Encoding]::new($false))

    if ($env:GITHUB_OUTPUT) {
        @("version=$version", "tag=$tag", "directory=$relativeDirectory", "artifact=$($config.name)-v$version-windows-amd64") |
            Add-Content -LiteralPath $env:GITHUB_OUTPUT -Encoding utf8
    }
    Write-Host "Release files: $releaseDirectory"
}
finally {
    [IO.File]::WriteAllBytes($configPath, $originalConfig)
    $env:CGO_ENABLED = $previousCGO
    Pop-Location
}
