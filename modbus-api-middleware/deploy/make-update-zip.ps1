param(
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$TargetOS,
    [Parameter(Mandatory = $true)][string]$TargetArch,
    [Parameter(Mandatory = $true)][string]$Binary,
    [Parameter(Mandatory = $true)][string]$Out
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path -LiteralPath $Binary)) { throw "Missing binary: $Binary" }
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("chpp-update-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $binName = Split-Path -Leaf $Binary
    Copy-Item -LiteralPath $Binary -Destination (Join-Path $tmp $binName)
    $stream = [System.IO.File]::OpenRead((Resolve-Path -LiteralPath $Binary).Path)
    try {
        $sha = [System.Security.Cryptography.SHA256]::Create()
        $hash = -join ($sha.ComputeHash($stream) | ForEach-Object { $_.ToString("x2") })
    }
    finally {
        $stream.Close()
    }
    $manifestJson = [ordered]@{
        app = "chpp-middleware"
        version = $Version
        os = $TargetOS
        arch = $TargetArch
        binary = $binName
        sha256 = $hash
    } | ConvertTo-Json
    [System.IO.File]::WriteAllText((Join-Path $tmp "update-manifest.json"), $manifestJson, [System.Text.UTF8Encoding]::new($false))
    $outDir = Split-Path -Parent $Out
    if ($outDir -and -not (Test-Path -LiteralPath $outDir)) { New-Item -ItemType Directory -Path $outDir | Out-Null }
    if (Test-Path -LiteralPath $Out) { Remove-Item -LiteralPath $Out -Force }
    Compress-Archive -Path (Join-Path $tmp "*") -DestinationPath $Out -Force
}
finally {
    if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Recurse -Force }
}


