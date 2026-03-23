[CmdletBinding()]
param(
    [switch]$All,
    [switch]$KeepDist
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

Write-Host '== SquareGuardian cleanup ==' -ForegroundColor Yellow
Push-Location $repoRoot

try {
    Invoke-DockerCommand -Arguments @('compose', 'down', '--remove-orphans', '--volumes')

    $storagePaths = @(
        'storage/events',
        'storage/frigate'
    )

    foreach ($path in $storagePaths) {
        if (Test-Path $path) {
            Get-ChildItem -Path $path -Force | Where-Object { $_.Name -ne '.gitkeep' } | Remove-Item -Recurse -Force
            Write-Host "Cleared $path" -ForegroundColor Yellow
        }
    }

    if ((-not $KeepDist) -and (Test-Path 'dist')) {
        Remove-Item 'dist' -Recurse -Force
        Write-Host 'Removed dist/' -ForegroundColor Yellow
    }

    if ($All) {
        Invoke-DockerCommand -Arguments @('builder', 'prune', '-af')
        Invoke-DockerCommand -Arguments @('system', 'prune', '-af', '--volumes')
    }

    Write-Host 'Cleanup complete.' -ForegroundColor Green
}
finally {
    Pop-Location
}
