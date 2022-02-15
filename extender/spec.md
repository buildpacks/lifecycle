# Usage - Extender

#### `extender`

Usage:

```
/cnb/lifecycle/extender \
  [-app <app>] \
  [-cache-dir <cache-dir>] \
  [-config <config> ] \
  [-ignore-paths <ignore-paths>] \
  [-kind <kind>] \
  [-log-level <log-level>] \
  [-work-dir <work-dir>] \
  <base-image> \
  <target-image>
```

##### Inputs

| Input            | Environment Variable      | Default Value            | Description                                                     |
|------------------|---------------------------|--------------------------|-----------------------------------------------------------------|
| `<app>`          | `CNB_APP_DIR`             | `/workspace`             | Path to application directory                                   |
| `<cache-dir>`    | `CNB_CACHE_DIR`           |                          | Path to a cache directory                                       |
| `<config>`       | `CNB_EXTEND_CONFIG_PATH`  | `<work-dir>/config.toml` | Path to a config file (see [`config.toml`](#extend-config-toml) |
| `<ignore-paths>` | `CNB_EXTEND_IGNORE_PATHS` |                          | Comma separated list of paths to ignore                         |
| `<base-image>`   |                           |                          | Base image to extend                                            |
| `<kind>`         |                           | `build`                  | Type of base image to extend (valid values: `build`, `run`)     |
| `<log-level>`    | `CNB_LOG_LEVEL`           | `info`                   | Log Level                                                       |
| `<target-image>` |                           |                          | Target image reference                                          |
| `<work-dir>`     | `CNB_EXTEND_WORK_DIR`     |                          | Path to a working directory                                     |

- `<base-image>` MUST be a valid image reference
- `<target-image>` MUST be a valid image reference

##### Outputs

| Output           | Description                |
|------------------|----------------------------|
| `[exit status]`  | Success (0), or error (1+) |
| `/dev/stdout`    | Logs (info)                |
| `/dev/stderr`    | Logs (warnings, errors)    |
| `<target-image>` | Extended base image        |

| Exit Code       | Result                             |
|-----------------|------------------------------------|
| `0`             | Success                            |
| `11`            | Platform API incompatibility error |
| `1-10`, `13-19` | Generic lifecycle errors           |
| `90-99`         | Extend-specific lifecycle errors   |
