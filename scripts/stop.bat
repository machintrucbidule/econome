@echo off
REM Stop the local EconoMe dev process (technical/07 §7).
taskkill /IM econome.exe /F >nul 2>&1
if %ERRORLEVEL%==0 (echo EconoMe stopped.) else (echo EconoMe was not running.)
