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

if exist detect-status-%bp_id%-%bp_version% (
  for /f "tokens=* USEBACKQ" %%F in (`type detect-status-%bp_id%-%bp_version%`) do (
    exit /b %%F
  )
)

exit /b 0
