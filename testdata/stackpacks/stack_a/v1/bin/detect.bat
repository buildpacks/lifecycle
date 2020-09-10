set layers_dir=%1
set platform_dir=%2
set plan_path=%3

set bp_dir=%~dp0..

if exist detect-status-%bp_id%-%bp_version% (
  for /f "tokens=* USEBACKQ" %%F in (`type detect-status-%bp_id%-%bp_version%`) do (
    exit /b %%F
  )
)