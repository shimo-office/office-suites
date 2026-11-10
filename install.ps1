# shimo-net CLI 安装（Windows PowerShell）
$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$destDir = Join-Path $env:LOCALAPPDATA "shimo-net"
$dest = Join-Path $destDir "shimo-net.exe"
New-Item -ItemType Directory -Force -Path $destDir | Out-Null

if (Get-Command go -ErrorAction SilentlyContinue) {
    Write-Host "使用 Go 构建：$(go version)"
    Push-Location $repoRoot
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags "-s -w" -o $dest ./cli
    Pop-Location
} else {
    $url = "https://github.com/shimo-office/office-suites/releases/latest/download/shimo-net_windows_$arch.exe"
    Write-Host "未找到 Go，下载预编译二进制：$url"
    try {
        Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
    } catch {
        Write-Error "下载失败。请安装 Go 1.22+ 后重试，或手动从 release 页面下载：$url"
        exit 1
    }
}

# 加入当前用户 PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$destDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$destDir;$userPath", "User")
    Write-Host "已把 $destDir 加入用户 PATH（新终端生效）"
}

# 如存在 DSH，安装技能
$skillRoot = Join-Path $env:USERPROFILE ".dsh\skills\shimo-net-cli"
if (Test-Path (Join-Path $env:USERPROFILE ".dsh")) {
    New-Item -ItemType Directory -Force -Path $skillRoot | Out-Null
    Copy-Item (Join-Path $repoRoot "cli\SKILL.md") (Join-Path $skillRoot "SKILL.md") -Force
    Write-Host "技能：$skillRoot"
}

Write-Host ""
Write-Host "== 完成 =="
& $dest doctor
Write-Host "首次使用：shimo-net init; shimo-net login"
