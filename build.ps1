param(
    [ValidateSet("amd64", "arm64", "386")]
    [string]$Architecture = "amd64"
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

$oldGOARCH = $env:GOARCH

Push-Location $root
try {
    Write-Host "Overuje se ESSENS Provisioner..."
    go test ./...
    go vet ./...

    $env:GOARCH = $Architecture

    $outputName = if ($Architecture -eq "amd64") {
        "ESSProvisioner.exe"
    }
    else {
        "ESSProvisioner-$Architecture.exe"
    }

    $outputPath = Join-Path $dist $outputName
    Write-Host "Sestavuje se GUI aplikace Windows pro architekturu $Architecture..."
    go build -trimpath -ldflags "-s -w -H=windowsgui" -o $outputPath ./cmd/essprovisioner

    Write-Host "Vytvoreno: $outputPath"
}
finally {
    Pop-Location
    if ($null -eq $oldGOARCH) {
        Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    }
    else {
        $env:GOARCH = $oldGOARCH
    }
}
