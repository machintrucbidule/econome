@echo off
REM EconoMe local Windows dev launcher (technical/07 §7).
REM Builds econome.exe if missing, points the data dir at a local ./data folder,
REM disables Secure cookies so login works over http://localhost, and runs it.
setlocal
set "ROOT=%~dp0.."
set "ECONOME_DATA_DIR=%ROOT%\data"
set "ECONOME_BEHIND_TLS=0"
set "ECONOME_DEFAULT_LOCALE=fr"
set "ECONOME_LOG_LEVEL=debug"

if not exist "%ECONOME_DATA_DIR%" mkdir "%ECONOME_DATA_DIR%"

if not exist "%ROOT%\econome.exe" (
  echo Building econome.exe ...
  pushd "%ROOT%"
  go build -o econome.exe ./cmd/econome || (echo Build failed & popd & exit /b 1)
  popd
)

echo Starting EconoMe on http://localhost:8765  (Ctrl+C to stop)
start "" http://localhost:8765
"%ROOT%\econome.exe"
