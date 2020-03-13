use failure::Error;

mod build_metadata;
mod env;
mod launcher;
use crate::{
    build_metadata::{BuildMetadata, Buildpack, Process},
    env::{Env, Position},
    launcher::Launcher,
};
use std::{env::var, path::Path};

const CODE_FAILED_LAUNCH: i32 = 8;

const DEFAULT_APP_DIR: &str = "/workspace";
const DEFAULT_LAYERS_DIR: &str = "/layers";
const DEFAULT_PROCESS_TYPE: &str = "web";

const ENV_APP_DIR: &str = "CNB_APP_DIR";
const ENV_LAYERS_DIR: &str = "CNB_LAYERS_DIR";
const ENV_PROCESS_TYPE: &str = "CNB_PROCESS_TYPE";

fn main() {
    if launch().is_err() {
        std::process::exit(CODE_FAILED_LAUNCH);
    }
}

fn launch() -> Result<(), Error> {
    let default_process_type = var(ENV_PROCESS_TYPE).unwrap_or(DEFAULT_PROCESS_TYPE.to_string());
    let layers_dir_string = var(ENV_LAYERS_DIR).unwrap_or(DEFAULT_LAYERS_DIR.to_string());
    let layers_dir = Path::new(&layers_dir_string);
    let app_dir_string = var(ENV_APP_DIR).unwrap_or(DEFAULT_APP_DIR.to_string());
    let app_dir = Path::new(&app_dir_string);
    let build_metadata: BuildMetadata = toml::from_str(&std::fs::read_to_string(
        layers_dir.join("config").join("metadata.toml"),
    )?)?;

    let mut launcher = Launcher::new(
        app_dir,
        layers_dir,
        default_process_type,
        build_metadata.buildpacks,
        env::OsEnv,
    );
    launcher.launch(build_metadata.processes)?;

    Ok(())
}
