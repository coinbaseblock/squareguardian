[CmdletBinding()]
param(
    [switch]$Clean,
    [switch]$Export = $true,
    [switch]$NoCache
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

function Invoke-DockerCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    Write-Host "> docker $($Arguments -join ' ')" -ForegroundColor Cyan
    & docker @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "docker $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

Write-Host '== SquareGuardian local start ==' -ForegroundColor Green
Push-Location $repoRoot

try {
    if (-not (Test-Path '.env')) {
        if (Test-Path '.env.example') {
            Copy-Item '.env.example' '.env'
            Write-Host 'Created .env from .env.example. Please review camera RTSP values before production use.' -ForegroundColor Yellow
        }
        else {
            throw 'Missing .env and .env.example'
        }
    }

    if ($Clean) {
        Write-Host 'Cleaning old project containers/images before rebuild...' -ForegroundColor Yellow
        Invoke-DockerCommand -Arguments @('compose', 'down', '--remove-orphans', '--volumes')
        Invoke-DockerCommand -Arguments @('builder', 'prune', '-af')
    }

    if ($Export) {
        New-Item -ItemType Directory -Force -Path 'dist' | Out-Null
        Invoke-DockerCommand -Arguments @('build', '--target', 'export', '-o', 'dist', '.')
    }

    $composeArgs = @('compose', 'up', '-d', '--build')
    if ($NoCache) {
        $composeArgs += '--no-cache'
    }
    Invoke-DockerCommand -Arguments $composeArgs
    Invoke-DockerCommand -Arguments @('compose', 'ps')

    Write-Host ''
    Write-Host '+----------------------------------------------------------+' -ForegroundColor Cyan
    Write-Host '|           SquareGuardian - Service Endpoints             |' -ForegroundColor Cyan
    Write-Host '+----------------------------------------------------------+' -ForegroundColor Cyan
    Write-Host '|  Web UI                                                  |' -ForegroundColor Cyan
    Write-Host '|    Frigate UI          http://localhost:8971             |' -ForegroundColor Green
    Write-Host '|    Detector Dashboard  http://localhost:8080             |' -ForegroundColor Green
    Write-Host '|    go2rtc Debug        http://localhost:1984             |' -ForegroundColor Green
    Write-Host '+----------------------------------------------------------+' -ForegroundColor Cyan
    Write-Host '|  API (from host)                                         |' -ForegroundColor Cyan
    Write-Host '|    Frigate API         http://localhost:5001             |' -ForegroundColor Green
    Write-Host '|    Detector API        http://localhost:8080             |' -ForegroundColor Green
    Write-Host '|    Face Service        http://localhost:8082  (optional) |' -ForegroundColor Green
    Write-Host '+----------------------------------------------------------+' -ForegroundColor Cyan
    Write-Host '|  Inter-container (Docker network)                        |' -ForegroundColor Cyan
    Write-Host '|    Frigate             http://frigate:5000               |' -ForegroundColor DarkGray
    Write-Host '|    Face Service        http://face-service:8082          |' -ForegroundColor DarkGray
    Write-Host '+----------------------------------------------------------+' -ForegroundColor Cyan
    Write-Host '|  RTSP Streams                                            |' -ForegroundColor Cyan
    Write-Host '|    RTSP (TCP)          rtsp://localhost:8554             |' -ForegroundColor DarkGray
    Write-Host '|    RTSP (TCP+UDP)      rtsp://localhost:8555             |' -ForegroundColor DarkGray
    Write-Host '+----------------------------------------------------------+' -ForegroundColor Cyan
    Write-Host ''
    Write-Host 'Use port 8971 for the Frigate web UI to avoid broken Explore thumbnails.' -ForegroundColor Yellow
    Write-Host 'Face Service requires profile "identity": docker compose --profile identity up -d' -ForegroundColor Yellow
    Write-Host ''
    Write-Host 'Useful follow-up:' -ForegroundColor Green
    Write-Host '  docker compose logs -f frigate'
    Write-Host '  docker compose logs -f squareguardian'
}
finally {
    Pop-Location
}
