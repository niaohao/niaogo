# Patch NieoData/config/ServerR.xml login IP/port for local.
# Usage: powershell -File scripts\patch_serverr_local.ps1

param(
  [string]$HostIP = "",
  [int]$LoginPort = 22973,
  [string]$NieoData = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if (-not $NieoData) {
  $NieoData = Join-Path (Split-Path -Parent $root) "NieoData"
}
$advFile = Join-Path $root "configs\advertise_host.txt"
if (-not $HostIP -and (Test-Path $advFile)) {
  $raw = [System.IO.File]::ReadAllText($advFile)
  $raw = $raw.TrimStart([char]0xFEFF)
  $HostIP = ($raw -split "`r?`n")[0].Trim()
}
if (-not $HostIP) { $HostIP = "127.0.0.1" }

$xmlPath = Join-Path $NieoData "config\ServerR.xml"
if (-not (Test-Path $xmlPath)) {
  Write-Error "missing $xmlPath"
}

$bak = "$xmlPath.bak_niao"
if (-not (Test-Path $bak)) {
  Copy-Item $xmlPath $bak
  Write-Host "[ok] backup -> $bak"
}

$text = [System.IO.File]::ReadAllText($xmlPath)
$text = [regex]::Replace($text, '(<(?:Email|DirSer|Visitor|SubServer|RegistSer)\s+ip=")[^"]+("\s+port=")[^"]+(")', {
  param($m)
  $m.Groups[1].Value + $HostIP + $m.Groups[2].Value + $LoginPort + $m.Groups[3].Value
})

$utf8NoBom = New-Object System.Text.UTF8Encoding $false
[System.IO.File]::WriteAllText($xmlPath, $text, $utf8NoBom)
Write-Host "[ok] patched $xmlPath"
Write-Host "     login endpoints -> ${HostIP}:${LoginPort}"
Write-Host "     open client via http://${HostIP}:41520/"
