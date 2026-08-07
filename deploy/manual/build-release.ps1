param(
    [string]$OutputDirectory,
    [string]$PublicGatewayUrl = "https://ygate.yokogawasolution.com",
    [string]$ReleaseVersion = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Invoke-Checked {
    param([scriptblock]$Command)

    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed: $Command"
    }
}

if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
    throw "Use this script on Windows. Linux builds use build-release.sh."
}

Get-Command git, go, node, npm.cmd, tar.exe -ErrorAction Stop | Out-Null

$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
if (-not $OutputDirectory) {
    $OutputDirectory = Join-Path $root "dist\manual"
}
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)

$releaseSha = (& git -C $root rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or $releaseSha -notmatch '^[0-9a-f]{40}$') {
    throw "Cannot resolve the Git commit SHA."
}

$timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$isDirty = [bool](& git -C $root status --porcelain)
$ciVersion = if ($ReleaseVersion) { $ReleaseVersion } elseif ($env:YGATE_RELEASE_VERSION) { $env:YGATE_RELEASE_VERSION } elseif ($env:BUILD_TAG) { $env:BUILD_TAG } elseif ($env:BUILD_NUMBER) { "jenkins-$($env:BUILD_NUMBER)-$($releaseSha.Substring(0, 12))" } else { "" }
if ($ciVersion) {
    $releaseId = $ciVersion.Trim()
    if ($releaseId -notmatch '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$') {
        throw "ReleaseVersion must contain only letters, numbers, dot, underscore, and hyphen."
    }
} else {
    $releaseId = if ($isDirty) { "${releaseSha}-dirty-$timestamp" } else { $releaseSha }
    if ($isDirty) {
        Write-Warning "Building a dirty worktree; the release ID is $releaseId"
    }
}

$workDirectory = Join-Path ([IO.Path]::GetTempPath()) ("ygate-" + [guid]::NewGuid().ToString("N"))
$releaseDirectory = Join-Path $workDirectory "release"
$oldCgo = $env:CGO_ENABLED
$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH
$oldGatewayUrl = $env:NEXT_PUBLIC_GATEWAY_URL
$oldAppVersion = $env:NEXT_PUBLIC_APP_VERSION

try {
    New-Item -ItemType Directory -Force -Path (Join-Path $releaseDirectory "bin"), (Join-Path $releaseDirectory "web"), $OutputDirectory | Out-Null

    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    Push-Location (Join-Path $root "services\platform-api")
    try {
        Invoke-Checked { go test ./... }
        Invoke-Checked { go build -trimpath -ldflags "-s -w -X main.version=$releaseId" -o (Join-Path $releaseDirectory "bin\platform-api.exe") ./cmd/platform-api }
        Invoke-Checked { go build -trimpath -ldflags "-s -w" -o (Join-Path $releaseDirectory "bin\platform-admin.exe") ./cmd/platform-admin }
    } finally {
        Pop-Location
    }

    Push-Location (Join-Path $root "services\api-gateway")
    try {
        Invoke-Checked { go test ./... }
        Invoke-Checked { go build -trimpath -ldflags "-s -w" -o (Join-Path $releaseDirectory "bin\api-gateway.exe") ./cmd/api-gateway }
    } finally {
        Pop-Location
    }

    Push-Location (Join-Path $root "services\auth-service")
    try {
        Invoke-Checked { go test ./... }
        Invoke-Checked { go build -trimpath -ldflags "-s -w -X main.version=$releaseId" -o (Join-Path $releaseDirectory "bin\auth-service.exe") ./cmd/auth-service }
    } finally {
        Pop-Location
    }

    $env:NEXT_PUBLIC_GATEWAY_URL = $PublicGatewayUrl
    $env:NEXT_PUBLIC_APP_VERSION = $releaseId
    Push-Location (Join-Path $root "apps\web")
    try {
        Invoke-Checked { npm.cmd ci }
        Invoke-Checked { npm.cmd run typecheck }
        Invoke-Checked { npm.cmd run build }
    } finally {
        Pop-Location
    }

    Copy-Item -Path (Join-Path $root "apps\web\.next\standalone\*") -Destination (Join-Path $releaseDirectory "web") -Recurse -Force
    Copy-Item -LiteralPath (Join-Path $root "packages\api-contracts\platform-api.yaml") -Destination $releaseDirectory
    Set-Content -LiteralPath (Join-Path $releaseDirectory "VERSION") -Value $releaseId -Encoding ascii

    @'
const path = require("node:path");
const root = __dirname;
const sharedEnv = {
  YGATE_ENV_FILE: process.env.YGATE_ENV_FILE || path.join(root, "ygate.env"),
  PLATFORM_OPENAPI_FILE: path.join(root, "platform-api.yaml"),
};

module.exports = {
  apps: [
    {
      name: "ygate-platform-api",
      script: path.join(root, "bin", "platform-api.exe"),
      interpreter: "none",
      cwd: root,
      env: sharedEnv,
      restart_delay: 5000,
    },
    {
      name: "ygate-api-gateway",
      script: path.join(root, "bin", "api-gateway.exe"),
      interpreter: "none",
      cwd: root,
      env: sharedEnv,
      restart_delay: 5000,
    },
    {
      name: "ygate-auth-service",
      script: path.join(root, "bin", "auth-service.exe"),
      interpreter: "none",
      cwd: root,
      env: sharedEnv,
      restart_delay: 5000,
    },
    {
      name: "ygate-web",
      script: path.join(root, "web", "server.js"),
      cwd: path.join(root, "web"),
      env: { NODE_ENV: "production", HOSTNAME: "127.0.0.1", PORT: "8080" },
      restart_delay: 5000,
    },
  ],
};
'@ | Set-Content -LiteralPath (Join-Path $releaseDirectory "ecosystem.config.cjs") -Encoding utf8

    @'
# Edit before first start. Do not commit this file.
PLATFORM_DATABASE_URL=postgresql://<user>:<password>@127.0.0.1:5432/<database>
PLATFORM_HTTP_ADDR=127.0.0.1:44441
PLATFORM_COOKIE_SECURE=true
PLATFORM_WEBSOCKET_ORIGINS=ygate.yokogawasolution.com
AUTH_DATABASE_URL=postgresql://<user>:<password>@127.0.0.1:5432/<database>
AUTH_HTTP_ADDR=127.0.0.1:44442
AUTH_COOKIE_SECURE=true
AUTH_SMTP_ADDR=smtp.example.com:587
AUTH_SMTP_FROM=ygate@example.com
AUTH_SMTP_USERNAME=<smtp-user>
AUTH_SMTP_PASSWORD=<smtp-password>
AUTH_PASSWORD_RESET_URL=https://ygate.yokogawasolution.com
GATEWAY_HTTP_ADDR=127.0.0.1:44440
GATEWAY_PLATFORM_URL=http://127.0.0.1:44441
GATEWAY_AUTH_SERVICE_URL=http://127.0.0.1:44442
GATEWAY_ALLOWED_ORIGINS=https://ygate.yokogawasolution.com
'@ | Set-Content -LiteralPath (Join-Path $releaseDirectory "ygate.env.example") -Encoding utf8

    @'
param(
    [Parameter(Mandatory)][string]$EnvFile,
    [Parameter(Mandatory)][string]$Email,
    [Parameter(Mandatory)][string]$DisplayName,
    [Parameter(Mandatory)][string]$OrganizationCode,
    [Parameter(Mandatory)][string]$OrganizationName,
    [string]$Username = "admin",
    [string]$Role = "System Admin"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$env:YGATE_ENV_FILE = [IO.Path]::GetFullPath($EnvFile)
$env:PLATFORM_BOOTSTRAP_EMAIL = $Email
$env:PLATFORM_BOOTSTRAP_USERNAME = $Username
$env:PLATFORM_BOOTSTRAP_DISPLAY_NAME = $DisplayName
$env:PLATFORM_BOOTSTRAP_ORGANIZATION_CODE = $OrganizationCode
$env:PLATFORM_BOOTSTRAP_ORGANIZATION_NAME = $OrganizationName
$env:PLATFORM_BOOTSTRAP_ROLE = $Role
$password = Read-Host "Admin password (12-72 characters)" -AsSecureString
$env:PLATFORM_BOOTSTRAP_PASSWORD = [Net.NetworkCredential]::new("", $password).Password

try {
    & (Join-Path $PSScriptRoot "bin\platform-admin.exe") bootstrap-user
    if ($LASTEXITCODE -ne 0) { throw "Failed to create the initial administrator." }
} finally {
    Remove-Item Env:PLATFORM_BOOTSTRAP_PASSWORD -ErrorAction SilentlyContinue
}
'@ | Set-Content -LiteralPath (Join-Path $releaseDirectory "bootstrap-admin.ps1") -Encoding utf8

    @'
param([string]$EnvFile)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

if (-not $EnvFile) { $EnvFile = Join-Path $PSScriptRoot "ygate.env" }
$EnvFile = [IO.Path]::GetFullPath($EnvFile)
if (-not (Test-Path -LiteralPath $EnvFile)) {
    Copy-Item ".\ygate.env.example" $EnvFile
    throw "Created $EnvFile. Edit its database credentials, then run start.ps1 again."
}
if (Select-String -LiteralPath $EnvFile -SimpleMatch "<") {
    throw "Replace every <placeholder> in $EnvFile before starting."
}

Get-Command node, pm2.cmd -ErrorAction Stop | Out-Null
$env:YGATE_ENV_FILE = $EnvFile
& .\bin\platform-admin.exe migrate
if ($LASTEXITCODE -ne 0) { throw "Database migration failed." }
& pm2.cmd startOrRestart .\ecosystem.config.cjs --update-env
if ($LASTEXITCODE -ne 0) { throw "PM2 failed to start YGATE." }
& pm2.cmd save
if ($LASTEXITCODE -ne 0) { throw "PM2 failed to save the process list." }

$urls = @(
    "http://127.0.0.1:44441/readyz",
    "http://127.0.0.1:44442/readyz",
    "http://127.0.0.1:44440/gateway/healthz",
    "http://127.0.0.1:44440/readyz",
    "http://127.0.0.1:8080/"
)
foreach ($url in $urls) {
    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        try {
            Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 5 | Out-Null
            $ready = $true
            break
        } catch {
            Start-Sleep -Seconds 2
        }
    }
    if (-not $ready) { throw "Health check failed: $url" }
}

Write-Host "YGATE is running. Use 'pm2 logs' to view logs."
'@ | Set-Content -LiteralPath (Join-Path $releaseDirectory "start.ps1") -Encoding utf8

    $artifact = Join-Path $OutputDirectory "ygate-$releaseId.zip"
    $checksumFile = "$artifact.sha256"
    Remove-Item -LiteralPath $artifact, $checksumFile -Force -ErrorAction SilentlyContinue
    Invoke-Checked { tar.exe -a -c -f $artifact -C $releaseDirectory . }
    $hash = (Get-FileHash -LiteralPath $artifact -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -LiteralPath $checksumFile -Value "$hash  $([IO.Path]::GetFileName($artifact))" -Encoding ascii

    Write-Host "release:  $releaseId"
    Write-Host "artifact: $artifact"
    Write-Host "checksum: $checksumFile"
} finally {
    $env:CGO_ENABLED = $oldCgo
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
    $env:NEXT_PUBLIC_GATEWAY_URL = $oldGatewayUrl
    $env:NEXT_PUBLIC_APP_VERSION = $oldAppVersion
    if ($workDirectory.StartsWith([IO.Path]::GetTempPath(), [StringComparison]::OrdinalIgnoreCase) -and (Test-Path -LiteralPath $workDirectory)) {
        Remove-Item -LiteralPath $workDirectory -Recurse -Force
    }
}
