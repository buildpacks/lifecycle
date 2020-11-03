@echo off

set layers_dir=%1
set platform_dir=%2
set plan_path=%3

set bp_dir=%~dp0..

for /f "tokens=* USEBACKQ" %%F in (`type %bp_dir%\buildpack.toml ^| yj -t ^| jq -r .buildpack.id`) do (
  set bp_id=%%F
)

for /f "tokens=* USEBACKQ" %%F in (`type %bp_dir%\buildpack.toml ^| yj -t ^| jq -r .buildpack.version`) do (
  set bp_version=%%F
)

echo build out: %bp_id%@%bp_version%
echo build err: %bp_id%@%bp_version%>&2

echo TEST_ENV: %TEST_ENV%> build-info-%bp_id%-%bp_version%
call :echon %CNB_BUILDPACK_DIR%> build-env-cnb-buildpack-dir-%bp_id%-%bp_version%

mkdir build-env-%bp_id%-%bp_version%
xcopy /e /q %platform_dir%\env build-env-%bp_id%-%bp_version% >nul
if %ERRORLEVEL% neq 0 (
  exit /b 1
)

type %plan_path% > build-plan-in-%bp_id%-%bp_version%.toml

if exist build-plan-out-%bp_id%-%bp_version%.toml (
  type build-plan-out-%bp_id%-%bp_version%.toml > %plan_path%
)

if exist build-%bp_id%-%bp_version%.toml (
  type build-%bp_id%-%bp_version%.toml > %layers_dir%\build.toml
)

if exist launch-%bp_id%-%bp_version%.toml (
  type launch-%bp_id%-%bp_version%.toml > %layers_dir%\launch.toml
)

if exist layers-%bp_id%-%bp_version% (
  xcopy /e /q layers-%bp_id%-%bp_version% %layers_dir% >nul
)

if exist build-status-%bp_id%-%bp_version% (
  for /f "tokens=* USEBACKQ" %%F in (`type build-status-%bp_id%-%bp_version%`) do (
    exit /b %%F
  )
)

if exist build-status (
  for /f "tokens=* USEBACKQ" %%F in (`type build-status`) do (
    exit /b %%F
  )
)

exit /b 0

:: echoes without newline
:echon
echo|set /p=%*
exit /b 0
