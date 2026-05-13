@echo off
setlocal
where pwsh >nul 2>nul
if %errorlevel%==0 (
  pwsh -NoLogo -NoProfile -Command "irm https://raw.githubusercontent.com/LFroesch/runx/main/install.ps1 | iex"
) else (
  powershell -NoLogo -NoProfile -Command "irm https://raw.githubusercontent.com/LFroesch/runx/main/install.ps1 | iex"
)
