param(
    [Parameter(Mandatory = $true)]
    [string]$Ffmpeg
)

$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$output = Join-Path $repo 'android\app\src\androidTest\assets\fixture.mp4'
New-Item -ItemType Directory -Force -Path (Split-Path $output) | Out-Null

& $Ffmpeg -hide_banner -loglevel error -y `
    -f lavfi -i 'testsrc2=size=320x180:rate=24' `
    -f lavfi -i 'sine=frequency=880:sample_rate=48000' `
    -t 4 -c:v libx264 -profile:v baseline -level 3.0 -pix_fmt yuv420p `
    -g 24 -keyint_min 24 -sc_threshold 0 -c:a aac -b:a 64k `
    -movflags +faststart -shortest $output

if ($LASTEXITCODE -ne 0) { throw "ffmpeg failed with exit code $LASTEXITCODE" }
Write-Output "Generated $output"
