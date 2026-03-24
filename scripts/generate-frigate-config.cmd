@echo off
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0generate-frigate-config.ps1" %*
