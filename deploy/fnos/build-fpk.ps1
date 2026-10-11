<#
.SYNOPSIS
    构建飞牛 fnOS 应用包（.fpk）。

.DESCRIPTION
    把 deploy/fnos/fpk 复制到临时目录，写入发布版本号与镜像标签，再调用官方 fnpack 打包。
    工作区内的包文件不会被修改，产物默认写到 dist/fnos/。

.EXAMPLE
    pwsh deploy/fnos/build-fpk.ps1 -Version 1.2.0
#>
[CmdletBinding()]
param(
    [string] $Version = '0.0.0',
    [string] $ImageTag = '',
    [string] $OutDir = '',
    [string] $Fnpack = '',
    [switch] $KeepStaging
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$packageDir = Join-Path $PSScriptRoot 'fpk'
if (-not (Test-Path -LiteralPath $packageDir)) { throw "找不到应用包目录：$packageDir" }

if ([string]::IsNullOrWhiteSpace($ImageTag)) {
    $ImageTag = if ($Version -eq '0.0.0') { 'latest' } else { $Version }
}
if ([string]::IsNullOrWhiteSpace($OutDir)) { $OutDir = Join-Path $repoRoot 'dist\fnos' }

# 官方只发布了 windows-amd64 与 linux-amd64 两个 fnpack，版本与校验和需和 build-fpk.sh 保持一致。
$fnpackVersion = '1.2.3'
$fnpackSha256 = 'D7AF4BD716B009C58F5BCD931615F39DB121E7D4B75DC759E575C4FB2879B6EE'
$fnpackUrl = "https://static2.fnnas.com/fnpack/fnpack-$fnpackVersion-windows-amd64"

if ([string]::IsNullOrWhiteSpace($Fnpack)) {
    $cacheDir = Join-Path $env:LOCALAPPDATA 'auto-anime-fpk'
    $Fnpack = Join-Path $cacheDir "fnpack-$fnpackVersion-windows-amd64.exe"
    if (-not (Test-Path -LiteralPath $Fnpack)) {
        New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null
        Write-Host "下载 fnpack $fnpackVersion ..."
        Invoke-WebRequest -Uri $fnpackUrl -OutFile $Fnpack -UseBasicParsing
    }
}
if (-not (Test-Path -LiteralPath $Fnpack)) { throw "找不到 fnpack：$Fnpack" }

$actualHash = (Get-FileHash -LiteralPath $Fnpack -Algorithm SHA256).Hash
if ($actualHash -ne $fnpackSha256) {
    throw "fnpack 校验失败：期望 $fnpackSha256，实际 $actualHash"
}

$stagingParent = Join-Path ([System.IO.Path]::GetTempPath()) ("auto-anime-fpk-" + [guid]::NewGuid().ToString('N'))
$staging = Join-Path $stagingParent 'auto-anime'
New-Item -ItemType Directory -Path $stagingParent -Force | Out-Null
Copy-Item -LiteralPath $packageDir -Destination $staging -Recurse -Force

# 生命周期脚本要在飞牛的 Linux 环境里执行，统一成 LF；顺带去掉 BOM。
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$textFiles = Get-ChildItem -LiteralPath $staging -Recurse -File | Where-Object {
    $_.FullName -match '[\\/](cmd|wizard)[\\/]' -or $_.Name -eq 'manifest' -or $_.Name -eq 'docker-compose.yaml' -or $_.Name -eq 'config' -or $_.Name -eq 'resource' -or $_.Name -eq 'privilege'
}
foreach ($file in $textFiles) {
    $text = [System.IO.File]::ReadAllText($file.FullName) -replace "`r`n", "`n"
    [System.IO.File]::WriteAllText($file.FullName, $text, $utf8NoBom)
}

$manifestPath = Join-Path $staging 'manifest'
$manifest = [System.IO.File]::ReadAllText($manifestPath)
$manifest = [regex]::Replace($manifest, '(?m)^version\s*=.*$', "version               = $Version")
[System.IO.File]::WriteAllText($manifestPath, $manifest, $utf8NoBom)

$composePath = Join-Path $staging 'app\docker\docker-compose.yaml'
$compose = [System.IO.File]::ReadAllText($composePath)
$compose = [regex]::Replace($compose, 'image:\s*ghcr\.io/shizuku11451/auto-anime:\S+', "image: ghcr.io/shizuku11451/auto-anime:$ImageTag")
[System.IO.File]::WriteAllText($composePath, $compose, $utf8NoBom)

Push-Location $staging
try {
    & $Fnpack build
    if ($LASTEXITCODE -ne 0) { throw "fnpack build 失败（退出码 $LASTEXITCODE）" }
}
finally {
    Pop-Location
}

$fpk = Join-Path $staging 'auto-anime.fpk'
if (-not (Test-Path -LiteralPath $fpk)) { throw '没有生成 auto-anime.fpk' }

New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
$target = Join-Path $OutDir "auto-anime-$Version-fnos.fpk"
Copy-Item -LiteralPath $fpk -Destination $target -Force

if (-not $KeepStaging) {
    $resolvedStaging = [System.IO.Path]::GetFullPath($stagingParent)
    $resolvedTemp = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolvedStaging.StartsWith($resolvedTemp, [System.StringComparison]::OrdinalIgnoreCase)) { throw "暂存目录不在临时目录内：$resolvedStaging" }
    Remove-Item -LiteralPath $resolvedStaging -Recurse -Force
}

Write-Host ''
Write-Host "已生成：$target"
Write-Host "版本：$Version    镜像：ghcr.io/shizuku11451/auto-anime:$ImageTag"
Write-Host '安装：飞牛应用中心 → 手动安装，选择上面的 .fpk，或执行 appcenter-cli install-fpk <文件>'
