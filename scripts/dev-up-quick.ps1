[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot

Write-Host '== SpaceGuardian quick start (no build) ==' -ForegroundColor Green
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

    Write-Host '> docker compose up -d' -ForegroundColor Cyan
    & docker compose up -d
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose up -d failed with exit code $LASTEXITCODE"
    }

    & docker compose ps

    Write-Host ''
    Write-Host 'Frigate UI:  http://localhost:8971' -ForegroundColor Green
    Write-Host 'Detector API: http://localhost:8080' -ForegroundColor Green
    Write-Host 'Frigate API:  http://localhost:5001' -ForegroundColor Green
    Write-Host ''
    Write-Host 'Note: No build was performed. Use dev-up.ps1 if you need to rebuild.' -ForegroundColor Yellow
}
finally {
    Pop-Location
}
