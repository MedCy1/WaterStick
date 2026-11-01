# WaterStick Installer for Windows PowerShell
$ErrorActionPreference = "Stop"

$Repo = "MedCy1/WaterStick"
$InstallDir = "$env:LOCALAPPDATA\Programs\WaterStick"

Write-Host "Checking for latest WaterStick release..."
$Headers = @{ "User-Agent" = "WaterStick-Installer" }
if ($env:GITHUB_TOKEN) {
    $Headers["Authorization"] = "Bearer $env:GITHUB_TOKEN"
} elseif ($env:GH_TOKEN) {
    $Headers["Authorization"] = "Bearer $env:GH_TOKEN"
}

try {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $Headers
    $Tag = $Release.tag_name
} catch {
    Write-Error "Failed to fetch latest release from GitHub: $_"
    exit 1
}

$ArchiveName = "waterstick_${Tag}_windows_amd64.zip"
$DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/$ArchiveName"

Write-Host "Downloading WaterStick $Tag (Windows AMD64)..."
$TempZip = [System.IO.Path]::GetTempFileName() + ".zip"
Invoke-WebRequest -Uri $DownloadUrl -Headers $Headers -OutFile $TempZip

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

Write-Host "Extracting to $InstallDir..."
Expand-Archive -Path $TempZip -DestinationPath $InstallDir -Force
Remove-Item -Path $TempZip -Force

# Add to user PATH if not present
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to user PATH."
}

# Create Desktop Shortcut
$WshShell = New-Object -ComObject WScript.Shell
$DesktopPath = [Environment]::GetFolderPath("Desktop")
$Shortcut = $WshShell.CreateShortcut("$DesktopPath\WaterStick.lnk")
$Shortcut.TargetPath = "$InstallDir\waterstick-tui.exe"
$Shortcut.Description = "WaterStick Dashboard"
$Shortcut.Save()

Write-Host "WaterStick $Tag installed successfully!"
Write-Host "  - Executable: $InstallDir\waterstick.exe"
Write-Host "  - Dashboard:  $InstallDir\waterstick-tui.exe"
Write-Host "  - Desktop shortcut created: $DesktopPath\WaterStick.lnk"
