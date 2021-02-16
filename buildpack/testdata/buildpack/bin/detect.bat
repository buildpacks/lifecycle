@echo off
set platform_dir=%1
set plan_path=%2

set bp_dir=%~dp0..

for /f "tokens=* USEBACKQ" %%F in (`type %bp_dir%\buildpack.toml ^| yj -t ^| jq -r .buildpack.id`) do (
  set bp_id=%%F
)

for /f "tokens=* USEBACKQ" %%F in (`type %bp_dir%\buildpack.toml ^| yj -t ^| jq -r .buildpack.version`) do (
  set bp_version=%%F
)

echo detect out: %bp_id%@%bp_version%
call :echon detect err: %bp_id%@%bp_version%>&2

dir /b %platform_dir%\env > detect-env-%bp_id%-%bp_version%
call :echon %ENV_TYPE%> detect-env-type-%bp_id%-%bp_version%
call :echon %CNB_BUILDPACK_DIR%> detect-env-cnb-buildpack-dir-%bp_id%-%bp_version%

if exist detect-plan-%bp_id%-%bp_version%.toml (
  type detect-plan-%bp_id%-%bp_version%.toml > %plan_path%
)

if exist detect-status-%bp_id%-%bp_version% (
  for /f "tokens=* USEBACKQ" %%F in (`type detect-status-%bp_id%-%bp_version%`) do (
    exit /b %%F
  )
)

if exist detect-status (
  for /f "tokens=* USEBACKQ" %%F in (`type detect-status`) do (
    exit /b %%F
  )
)

exit /b 0

:: echoes without newline
:echon
echo|set /p=%*
exit /b 0
