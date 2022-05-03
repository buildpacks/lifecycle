@echo off
set platform_dir=%1
set plan_path=%2

set bp_dir=%~dp0..

for /f "tokens=* USEBACKQ" %%F in (`type %bp_dir%\extension.toml ^| yj -t ^| jq -r .extension.id`) do (
  set bp_id=%%F
)

for /f "tokens=* USEBACKQ" %%F in (`type %bp_dir%\extension.toml ^| yj -t ^| jq -r .extension.version`) do (
  set bp_version=%%F
)

echo detect out: %bp_id%@%bp_version%
call :echon detect err: %bp_id%@%bp_version%>&2

if not defined CNB_PLATFORM_DIR ( set CNB_PLATFORM_DIR="unset" )
if not defined CNB_BUILD_PLAN_PATH ( set CNB_BUILD_PLAN_PATH="unset" )

dir /b %platform_dir%\env > detect-env-%bp_id%-%bp_version%
call :echon %ENV_TYPE%> detect-env-type-%bp_id%-%bp_version%
call :echon %CNB_BUILDPACK_DIR%> detect-env-cnb-buildpack-dir-%bp_id%-%bp_version%
call :echon %CNB_PLATFORM_DIR%> detect-env-cnb-platform-dir-%bp_id%-%bp_version%
call :echon %CNB_BUILD_PLAN_PATH%> detect-env-cnb-build-plan-path-%bp_id%-%bp_version%

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
