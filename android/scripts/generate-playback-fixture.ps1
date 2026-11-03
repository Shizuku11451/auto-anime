param(
    [Parameter(Mandatory = $true)]
    [string]$Ffmpeg,
    # The player keeps a resume position only past ten seconds, so the fixture must be long
    # enough for instrumented tests to seek beyond that threshold and reopen it.
    [int]$Seconds = 120
)

$ErrorActionPreference = 'Stop'
# Resolved relative to this script so the C:\aa-android junction also works.
$output = Join-Path $PSScriptRoot '..\app\src\androidTest\assets\fixture.mp4'
$output = (Resolve-Path (Split-Path $output)).Path + '\fixture.mp4'
New-Item -ItemType Directory -Force -Path (Split-Path $output) | Out-Null

& $Ffmpeg -hide_banner -loglevel error -y `
    -f lavfi -i 'testsrc2=size=320x180:rate=12' `
    -f lavfi -i 'sine=frequency=880:sample_rate=48000' `
    -t $Seconds -c:v libx264 -profile:v baseline -level 3.0 -pix_fmt yuv420p -b:v 150k `
    -g 24 -keyint_min 24 -sc_threshold 0 -c:a aac -b:a 48k `
    -movflags +faststart -shortest $output

if ($LASTEXITCODE -ne 0) { throw "ffmpeg failed with exit code $LASTEXITCODE" }
Write-Output "Generated $output ($Seconds s)"
