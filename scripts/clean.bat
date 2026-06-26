@echo off
REM EconoMe local dev — reset to a fresh database (technical/07 §7).
REM Deletes the local data folder (SQLite DB + session secret + backups).
REM IRREVERSIBLE — asks for explicit confirmation first.
setlocal
set "ROOT=%~dp0.."
set "DATADIR=%ROOT%\data"

if not exist "%DATADIR%" (
  echo No local data folder at "%DATADIR%" -- already clean.
  goto :eof
)

echo WARNING: this permanently deletes "%DATADIR%"
echo          (database, session secret, and local backups).
echo The next launch will start from the owner-creation wizard.
echo.
set /p "ANS=Type YES to confirm: "
if /I not "%ANS%"=="YES" (
  echo Aborted -- nothing deleted.
  goto :eof
)

REM Stop the app first so the SQLite file is not locked.
taskkill /IM econome.exe /F >nul 2>&1

rmdir /s /q "%DATADIR%"
if exist "%DATADIR%" (
  echo Failed to remove "%DATADIR%" -- is the app still running?
) else (
  echo Done -- fresh database. Run scripts\start.bat to create a new owner.
)
