<#
.SYNOPSIS
    frp 构建打包脚本 - 支持 Windows 和 Linux x86 平台
.DESCRIPTION
    提供菜单交互，选择目标平台后编译 frpc 和 frps 二进制文件。
    同时构建两个平台请依次执行。
    构建 Go 前会自动编译前端资源（web/frps 和 web/frpc）。
#>

$LDFLAGS = "-s -w"
$NOWEB_TAG = ",noweb"

function Build-Frontend {
    Write-Host ""
    Write-Host "==============================" -ForegroundColor Green
    Write-Host "  构建前端资源..." -ForegroundColor Green
    Write-Host "==============================" -ForegroundColor Green
    Write-Host ""

    Write-Host "[1/2] 构建 frps 面板 (web/frps)..." -ForegroundColor Magenta
    Push-Location "web/frps"
    try {
        $result = npm run build-only 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Host $result
            Write-Host "  [错误] frps 前端构建失败！" -ForegroundColor Red
            return $false
        }
    } finally {
        Pop-Location
    }
    Write-Host "  [完成] web/frps/dist/" -ForegroundColor Green

    Write-Host "[2/2] 构建 frpc 面板 (web/frpc)..." -ForegroundColor Magenta
    Push-Location "web/frpc"
    try {
        $result = npm run build-only 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Host $result
            Write-Host "  [错误] frpc 前端构建失败！" -ForegroundColor Red
            return $false
        }
    } finally {
        Pop-Location
    }
    Write-Host "  [完成] web/frpc/dist/" -ForegroundColor Green

    # 重新检查 dist 目录，更新 NOWEB_TAG
    if ((Test-Path "web/frps/dist" -PathType Container) -and (Test-Path "web/frpc/dist" -PathType Container)) {
        $script:NOWEB_TAG = ""
    }

    Write-Host ""
    Write-Host "  前端构建完成！" -ForegroundColor Green
    Write-Host ""
    return $true
}

function Show-Menu {
    Clear-Host
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host "          frp 构建打包脚本" -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  1. 构建 Windows 平台 (amd64)" -ForegroundColor Yellow
    Write-Host "  2. 构建 Linux x86 平台 (amd64)" -ForegroundColor Yellow
    Write-Host "  3. 构建所有平台 (Windows + Linux)" -ForegroundColor Yellow
    Write-Host "  4. 仅构建前端 (frps-web + frpc-web)" -ForegroundColor Yellow
    Write-Host "  Q. 退出" -ForegroundColor Yellow
    Write-Host ""
}

function Build-Platform {
    param (
        [string]$GOOS,
        [string]$GOARCH,
        [string]$Label
    )

    Write-Host ""
    Write-Host "==============================" -ForegroundColor Green
    Write-Host "  构建 ${Label}..." -ForegroundColor Green
    Write-Host "==============================" -ForegroundColor Green
    Write-Host ""

    # 设置交叉编译环境变量
    $env:GOOS = $GOOS
    $env:GOARCH = $GOARCH
    $env:CGO_ENABLED = 0
    $env:GO111MODULE = "on"

    # 确保 bin 目录存在
    if (-not (Test-Path "bin" -PathType Container)) {
        New-Item -ItemType Directory -Path "bin" -Force | Out-Null
    }

    # 确定输出文件名后缀
    $ext = ""
    if ($GOOS -eq "windows") {
        $ext = ".exe"
    }

    # 创建平台子目录
    $platformDir = "bin"
    $frpcName = "frpc${ext}"
    $frpsName = "frps${ext}"

    Write-Host "[1/2] 编译 frps..." -ForegroundColor Magenta
    $frpsCmd = "go build -trimpath -ldflags `"$LDFLAGS`" -tags `"frps${NOWEB_TAG}`" -o ${platformDir}/${frpsName} ./cmd/frps"
    Write-Host "  -> $frpsCmd" -ForegroundColor DarkGray
    Invoke-Expression $frpsCmd
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  [错误] frps 编译失败！" -ForegroundColor Red
        return $false
    }
    Write-Host "  [完成] ${platformDir}/${frpsName}" -ForegroundColor Green

    Write-Host "[2/2] 编译 frpc..." -ForegroundColor Magenta
    $frpcCmd = "go build -trimpath -ldflags `"$LDFLAGS`" -tags `"frpc${NOWEB_TAG}`" -o ${platformDir}/${frpcName} ./cmd/frpc"
    Write-Host "  -> $frpcCmd" -ForegroundColor DarkGray
    Invoke-Expression $frpcCmd
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  [错误] frpc 编译失败！" -ForegroundColor Red
        return $false
    }
    Write-Host "  [完成] ${platformDir}/${frpcName}" -ForegroundColor Green

    Write-Host ""
    Write-Host "==============================" -ForegroundColor Green
    Write-Host "  ${Label} 构建成功！" -ForegroundColor Green
    Write-Host "  输出目录: ${platformDir}/" -ForegroundColor Green
    Write-Host "==============================" -ForegroundColor Green
    return $true
}

# 主循环
do {
    Show-Menu
    $choice = Read-Host "请选择 (1/2/3/4/Q)"

    switch ($choice) {
        "1" {
            Build-Frontend
            if (-not $?) { break }
            Build-Platform -GOOS "windows" -GOARCH "amd64" -Label "Windows amd64"
            Write-Host ""
            Write-Host "按任意键返回菜单..." -ForegroundColor DarkGray
            $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        }
        "2" {
            Build-Frontend
            if (-not $?) { break }
            Build-Platform -GOOS "linux" -GOARCH "amd64" -Label "Linux amd64"
            Write-Host ""
            Write-Host "按任意键返回菜单..." -ForegroundColor DarkGray
            $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        }
        "3" {
            Write-Host ""
            Write-Host "========================================" -ForegroundColor Cyan
            Write-Host "  开始构建所有平台..." -ForegroundColor Cyan
            Write-Host "========================================" -ForegroundColor Cyan
            Build-Frontend
            if ($?) {
                Build-Platform -GOOS "windows" -GOARCH "amd64" -Label "Windows amd64"
                Write-Host ""
                Build-Platform -GOOS "linux" -GOARCH "amd64" -Label "Linux amd64"
            }
            Write-Host ""
            Write-Host "按任意键返回菜单..." -ForegroundColor DarkGray
            $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        }
        "4" {
            Build-Frontend
            Write-Host ""
            Write-Host "按任意键返回菜单..." -ForegroundColor DarkGray
            $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        }
        "Q" { }
        "q" { $choice = "Q" }
        default {
            Write-Host "无效选项，请重新选择。" -ForegroundColor Red
            Start-Sleep -Seconds 1
        }
    }
} while ($choice -ne "Q")
