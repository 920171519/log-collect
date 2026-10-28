param(
    [string]$Target = "windows/amd64"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectRoot

if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    throw "未找到 wails 命令。请先运行: go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0"
}

Write-Host "==> 构建 $Target" -ForegroundColor Cyan
wails build -platform $Target -clean -trimpath
if ($LASTEXITCODE -ne 0) {
    throw "wails build 失败"
}

$SourceExe = Join-Path $ProjectRoot "build\bin\LogCollector.exe"
if (-not (Test-Path $SourceExe)) {
    throw "未找到构建产物: $SourceExe"
}

$ReleaseDir = Join-Path $ProjectRoot "release\windows-amd64"
if (Test-Path $ReleaseDir) {
    Remove-Item -Recurse -Force $ReleaseDir
}
New-Item -ItemType Directory -Path $ReleaseDir -Force | Out-Null

Copy-Item $SourceExe (Join-Path $ReleaseDir "LogCollector.exe")
Copy-Item (Join-Path $ProjectRoot "config.example.json") (Join-Path $ReleaseDir "logcollector.config.json")

Write-Host ""
Write-Host "构建完成。最终目录只包含一个 exe 和一个配置文件:" -ForegroundColor Green
Get-ChildItem $ReleaseDir | Select-Object Name, Length | Format-Table
Write-Host "目录: $ReleaseDir"
