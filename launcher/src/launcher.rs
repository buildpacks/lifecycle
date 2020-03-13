use crate::Buildpack;
use crate::Process;
use crate::{Env, Position};
use failure::Error;
use std::{
    collections::HashMap,
    ffi::{CStr, CString},
    fs,
    path::{Path, PathBuf},
};

pub struct Launcher<E> {
    app_dir: PathBuf,
    layers_dir: PathBuf,
    buildpacks: Vec<Buildpack>,
    default_process_type: String,
    env: E,
}

impl<E: Env> Launcher<E> {
    pub fn new<A: Into<PathBuf>, L: Into<PathBuf>, D: Into<String>>(
        a: A,
        l: L,
        d: D,
        buildpacks: Vec<Buildpack>,
        env: E,
    ) -> Self {
        let app_dir = a.into();
        let layers_dir = l.into();
        let default_process_type = d.into();

        Self {
            app_dir,
            layers_dir,
            buildpacks,
            default_process_type,
            env,
        }
    }

    #[allow(dead_code)]
    pub fn launch(&mut self, mut processes: Vec<Process>) -> Result<(), Error> {
        let profile_d = walk_layers_dir(
            &mut self.env,
            &self.app_dir,
            &self.layers_dir,
            &self.buildpacks,
        )?;
        let process = detect_process(&[], &self.default_process_type, &mut processes)
            .ok_or("Can not find start command")
            .unwrap();
        std::env::set_current_dir(&self.app_dir)?;
        if process.direct {
            let path = CString::new(process.command)?;
            let args_owned: Vec<CString> = process
                .args
                .into_iter()
                // TODO: can we remove this unwrap?
                .map(|arg| CString::new(arg).unwrap())
                .collect();
            nix::unistd::execve(
                &path,
                &args_owned
                    .iter()
                    .map(|arg| arg.as_c_str())
                    .collect::<Vec<&CStr>>()[..],
                &self
                    .env
                    .list()
                    .iter()
                    .map(|arg| arg.as_c_str())
                    .collect::<Vec<&CStr>>()[..],
            )?;
        } else {
            let path = CString::new("/bin/bash")?;
            let mut args_owned: Vec<CString> = Vec::new();
            args_owned.push(CString::new("bash")?);
            args_owned.push(CString::new("-c")?);
            args_owned.append(
                &mut profile_d
                    .into_iter()
                    .map(|p| CString::new(p.into_os_string().into_string().unwrap()).unwrap())
                    .collect::<Vec<CString>>(),
            );
            args_owned.append(
                &mut process
                    .args
                    .into_iter()
                    .map(|arg| CString::new(arg).unwrap())
                    .collect::<Vec<CString>>(),
            );
            args_owned.push(CString::new("launcher")?);
            args_owned.push(CString::new(process.command)?);
            nix::unistd::execve(
                &path,
                &args_owned
                    .iter()
                    .map(|arg| arg.as_c_str())
                    .collect::<Vec<&CStr>>()[..],
                &self
                    .env
                    .list()
                    .iter()
                    .map(|arg| arg.as_c_str())
                    .collect::<Vec<&CStr>>()[..],
            )?;
        }

        Ok(())
    }
}

fn collect_layer_profile_d<P: AsRef<Path>>(layer_dir: P) -> Result<Vec<PathBuf>, Error> {
    let layer_dir_path = layer_dir.as_ref();
    let mut paths: Vec<PathBuf> = Vec::new();
    let profile_d_path = layer_dir_path.join("profile.d");

    if profile_d_path.is_dir() {
        for profile_d_file_entry in profile_d_path.read_dir()? {
            if let Ok(profile_d_file_entry) = profile_d_file_entry {
                let profile_d_file_path = profile_d_file_entry.path();
                if profile_d_file_path.is_file() {
                    paths.push(profile_d_file_path);
                }
            }
        }
    }

    // sort alphabetically ascending order by file name.
    paths.sort();

    Ok(paths)
}

fn walk_layers_dir<A: AsRef<Path>, L: AsRef<Path>>(
    env: &mut impl Env,
    a: A,
    l: L,
    buildpacks: &[Buildpack],
) -> Result<Vec<PathBuf>, Error> {
    let app_dir = a.as_ref();
    let layers_dir = l.as_ref();

    fs::metadata(app_dir)?;

    let posix_env: HashMap<&str, &str> = vec![("bin", "PATH"), ("lib", "LD_LIBRARY_PATH")]
        .into_iter()
        .collect();
    let env_dirs = ["env", "env.launch"];
    let mut profile_d: Vec<PathBuf> = Vec::new();

    for buildpack in buildpacks {
        let bp_layers_dir_path = layers_dir.join(buildpack.path_id());
        fs::metadata(&bp_layers_dir_path)?;
        if same_file::is_same_file(app_dir, &bp_layers_dir_path)? {
            continue;
        }
        // sort buildpack layers by alphabetical order
        let mut bp_layers: Vec<std::fs::DirEntry> = bp_layers_dir_path
            .read_dir()?
            .filter_map(|entry| entry.ok())
            .collect();
        bp_layers.sort_by_key(|dir| dir.path());
        for entry in bp_layers {
            let layer_path = entry.path();

            // add bin / lib dirs to env
            add_root_layer_dirs(env, &layer_path, &posix_env)?;
            // add env + env.launch contents to env
            add_env_layer_dirs(env, &layer_path, &env_dirs)?;
            // build profile.d script
            profile_d.append(&mut collect_layer_profile_d(&layer_path)?);
        }
    }

    Ok(profile_d)
}

fn add_root_layer_dirs<P: AsRef<Path>>(
    env: &mut impl Env,
    p: P,
    posix_env: &HashMap<&str, &str>,
) -> Result<(), Error> {
    let path = p.as_ref();

    for (dir_name, var_name) in posix_env {
        let dir_path = path.join(dir_name);
        if dir_path.is_dir() {
            env.modify_var(&var_name, Position::Prefix, &dir_path)?;
        }
    }

    Ok(())
}

fn add_env_layer_dirs<P: AsRef<Path>>(
    env: &mut impl Env,
    p: P,
    dirs: &[&str],
) -> Result<(), Error> {
    let path = p.as_ref();

    for dir_name in dirs.iter() {
        let env_dir_path = path.join(dir_name);
        if env_dir_path.is_dir() {
            for env_file_entry in env_dir_path.read_dir()? {
                if let Ok(env_file_entry) = env_file_entry {
                    let env_file_path = env_file_entry.path();

                    add_env_file(env, &env_file_path)?;
                }
            }
        }
    }

    Ok(())
}

fn add_env_file<P: AsRef<Path>>(env: &mut impl Env, p: P) -> Result<(), Error> {
    let path = p.as_ref();

    if !path.is_file() {
        return Ok(());
    }

    if let Some(var_name) = path.file_stem() {
        let value = std::fs::read_to_string(&path)?;

        match path.extension() {
            Some(ext) => {
                if ext == "prepend" {
                    env.modify_var(var_name, Position::Prefix, &value)?;
                } else if ext == "append" {
                    env.modify_var(var_name, Position::Suffix, &value)?;
                } else if ext == "override" || (ext == "default" && env.var_os(var_name).is_none())
                {
                    env.set_var(var_name, &value);
                }
            }
            None => env.modify_var(var_name, Position::Suffix, &value)?,
        }
    }

    Ok(())
}

fn detect_process(
    argv: &[String],
    default_process_type: &str,
    processes: &mut Vec<Process>,
) -> Option<Process> {
    if argv.is_empty() {
        return find_process_by_type(processes, default_process_type);
    } else if argv.len() == 1 {
        let process = find_process_by_type(processes, &argv[0]);
        if process.is_some() {
            return process;
        }
    } else if argv[0] == "--" {
        return Some(Process {
            r#type: "".to_string(),
            command: argv[1].clone(),
            args: argv[2..argv.len()].to_vec(),
            direct: true,
        });
    }

    Some(Process {
        r#type: "".to_string(),
        command: argv[0].clone(),
        args: argv[1..argv.len()].to_vec(),
        direct: false,
    })
}

fn find_process_by_type(processes: &mut Vec<Process>, r#type: &str) -> Option<Process> {
    processes.retain(|process| process.r#type == r#type);
    processes.pop()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::env::test_helpers::TestEnv;
    use std::{collections::HashMap, fs};
    use tempdir::TempDir;

    #[test]
    fn it_adds_dirs_to_env() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let posix_env: HashMap<&str, &str> = vec![("bin", "PATH"), ("lib", "LD_LIBRARY_PATH")]
            .into_iter()
            .collect();
        let tmpdir = TempDir::new("launcher")?;
        let bin_dir = tmpdir.path().join("bin");
        let lib_dir = tmpdir.path().join("lib");
        for dir in [&bin_dir, &lib_dir].iter() {
            fs::create_dir(dir)?
        }

        assert!(add_root_layer_dirs(&mut env, &tmpdir.path(), &posix_env).is_ok());
        assert_eq!(env.var_os("PATH").unwrap(), bin_dir);
        assert_eq!(env.var_os("LD_LIBRARY_PATH").unwrap(), lib_dir);

        Ok(())
    }

    #[test]
    fn it_adds_env_file_not_file() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let tmpdir = TempDir::new("launcher")?;
        let env_path = tmpdir.path().join("env");
        fs::create_dir(&env_path)?;

        assert!(add_env_file(&mut env, &env_path).is_ok());

        Ok(())
    }

    #[test]
    fn it_adds_env_file_prepend() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let tmpdir = TempDir::new("launcher")?;
        let var = "TEST";
        let env_path = tmpdir.path().join(format!("{}.prepend", var));
        env.set_var(var, "bar");
        fs::write(&env_path, "foo")?;

        assert!(add_env_file(&mut env, &env_path).is_ok());
        assert_eq!(env.var_os(var).unwrap(), "foo:bar");

        Ok(())
    }

    #[test]
    fn it_adds_env_file_append() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let tmpdir = TempDir::new("launcher")?;
        let var = "TEST";
        let env_path = tmpdir.path().join(format!("{}.append", var));
        env.set_var(var, "bar");
        fs::write(&env_path, "foo")?;

        assert!(add_env_file(&mut env, &env_path).is_ok());
        assert_eq!(env.var_os(var).unwrap(), "bar:foo");

        Ok(())
    }

    #[test]
    fn it_adds_env_file_override() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let tmpdir = TempDir::new("launcher")?;
        let var = "TEST";
        let env_path = tmpdir.path().join(format!("{}.override", var));
        env.set_var(var, "bar");
        fs::write(&env_path, "foo")?;

        assert!(add_env_file(&mut env, &env_path).is_ok());
        assert_eq!(env.var_os(var).unwrap(), "foo");

        Ok(())
    }

    #[test]
    fn it_adds_env_file_default_does_not_set_if_exists() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let tmpdir = TempDir::new("launcher")?;
        let var = "TEST";
        let env_path = tmpdir.path().join(format!("{}.default", var));
        env.set_var(var, "bar");
        fs::write(&env_path, "foo")?;

        assert!(add_env_file(&mut env, &env_path).is_ok());
        assert_eq!(env.var_os(var).unwrap(), "bar");

        Ok(())
    }

    #[test]
    fn it_adds_env_file_default_sets_if_not_exists() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let tmpdir = TempDir::new("launcher")?;
        let var = "TEST";
        let env_path = tmpdir.path().join(format!("{}.default", var));
        fs::write(&env_path, "foo")?;

        assert!(add_env_file(&mut env, &env_path).is_ok());
        assert_eq!(env.var_os(var).unwrap(), "foo");

        Ok(())
    }

    #[test]
    fn it_builds_profile_d_list() -> Result<(), Error> {
        let tmpdir = TempDir::new("launcher")?;
        let profiled_path = tmpdir.path().join("profile.d");
        fs::create_dir_all(&profiled_path)?;
        let foo_path = profiled_path.join("foo");
        let bar_path = profiled_path.join("bar");
        fs::write(&foo_path, "foo")?;
        fs::write(&bar_path, "bar")?;
        fs::create_dir_all(profiled_path.join("baz"))?;

        let result = collect_layer_profile_d(tmpdir.path());
        assert!(result.is_ok());
        if let Ok(profile_ds) = result {
            assert_eq!(profile_ds, [bar_path, foo_path].to_vec());
        }

        Ok(())
    }

    fn web_process() -> Process {
        Process {
            r#type: "web".to_string(),
            command: "bin/rails".to_string(),
            args: ["-p", "$PORT"].iter().map(|&s| s.to_string()).collect(),
            direct: false,
        }
    }

    fn worker_process() -> Process {
        Process {
            r#type: "worker".to_string(),
            command: "bundle exec sidekiq".to_string(),
            args: ["-c", "config/sidekiq.yml"]
                .iter()
                .map(|&s| s.to_string())
                .collect(),
            direct: false,
        }
    }

    #[test]
    fn it_finds_process_by_type() {
        let mut processes = Vec::new();
        let web = web_process();
        let worker = worker_process();
        processes.push(web.clone());
        processes.push(worker);

        assert_eq!(find_process_by_type(&mut processes, "web"), Some(web));
    }

    #[test]
    fn it_detects_process_for_default() {
        let mut processes = Vec::new();
        let web = web_process();
        processes.push(web.clone());

        assert_eq!(detect_process(&[], "web", &mut processes), Some(web));
    }

    #[test]
    fn it_detects_process_for_default_if_none() {
        let mut processes = Vec::new();

        assert_eq!(detect_process(&[], "web", &mut processes), None);
    }

    #[test]
    fn it_detects_process_by_process_type_name() {
        let mut processes = Vec::new();
        let worker = worker_process();
        processes.push(worker.clone());

        assert_eq!(
            detect_process(&["worker".to_string()], "web", &mut processes),
            Some(worker)
        );
    }

    #[test]
    fn it_detects_process_as_process_command_if_no_process_type() {
        let mut processes = Vec::new();

        assert_eq!(
            detect_process(&["bash".to_string()], "web", &mut processes),
            Some(Process {
                r#type: "".to_string(),
                command: "bash".to_string(),
                args: Vec::new(),
                direct: false,
            })
        );
    }

    #[test]
    fn it_detects_process_as_direct() {
        let mut processes = Vec::new();

        assert_eq!(
            detect_process(
                &["--", "bin/rails", "start"]
                    .iter()
                    .map(|&s| s.to_string())
                    .collect::<Vec<String>>(),
                "web",
                &mut processes
            ),
            Some(Process {
                r#type: "".to_string(),
                command: "bin/rails".to_string(),
                args: ["start".to_string()].to_vec(),
                direct: true,
            })
        );
    }

    #[test]
    fn it_detects_process_for_any_command() {
        let mut processes = Vec::new();

        assert_eq!(
            detect_process(
                &["bin/rails", "start"]
                    .iter()
                    .map(|&s| s.to_string())
                    .collect::<Vec<String>>(),
                "web",
                &mut processes
            ),
            Some(Process {
                r#type: "".to_string(),
                command: "bin/rails".to_string(),
                args: ["start".to_string()].to_vec(),
                direct: false,
            })
        );
    }

    #[test]
    fn it_walks_layers_dir() -> Result<(), Error> {
        let mut env = TestEnv::new();
        let tmpdir = TempDir::new("launcher")?;
        let app_dir = tmpdir.path().join("workspace");
        let layers_dir = tmpdir.path().join("layers");
        let ruby_buildpack = Buildpack::new("heroku/ruby", "1.0.0");
        let ruby_path = layers_dir.join(&ruby_buildpack.path_id());
        let ruby_layer_path = ruby_path.join("ruby");
        let gems_layer_path = ruby_path.join("gems");
        let procfile_buildpack = Buildpack::new("heroku/procfile", "1.0.0");
        let procfile_path = layers_dir.join(&procfile_buildpack.path_id());
        let tools_layer_path = procfile_path.join("tools");
        let buildpacks = [ruby_buildpack, procfile_buildpack];
        let ruby_profile_d_path = ruby_layer_path.join("profile.d");
        let foo_profile_d_path = ruby_profile_d_path.join("foo.sh");
        let bar_profile_d_path = ruby_profile_d_path.join("bar.sh");
        let gems_profile_d_path = gems_layer_path.join("profile.d");
        let baz_profile_d_path = gems_profile_d_path.join("baz.sh");
        let tools_profile_d_path = tools_layer_path.join("profile.d");
        let far_profile_d_path = tools_profile_d_path.join("far.sh");

        fs::create_dir_all(&app_dir)?;
        fs::create_dir_all(&ruby_layer_path.join("bin"))?;
        fs::create_dir_all(&ruby_layer_path.join("lib"))?;
        fs::create_dir_all(&ruby_layer_path.join("env"))?;
        fs::create_dir_all(&ruby_layer_path.join("env.launch"))?;
        fs::create_dir_all(&procfile_path)?;
        fs::create_dir_all(&tools_layer_path.join("bin"))?;
        fs::create_dir_all(&tools_layer_path.join("lib"))?;
        fs::create_dir_all(&ruby_profile_d_path)?;
        fs::create_dir_all(&gems_profile_d_path)?;
        fs::create_dir_all(&tools_profile_d_path)?;

        fs::write(&ruby_layer_path.join("env.launch").join("FOO"), "foo")?;
        fs::write(&ruby_layer_path.join("env").join("PATH"), "vendor/ruby/bin")?;
        fs::write(&foo_profile_d_path, "export FOO=foo")?;
        fs::write(&bar_profile_d_path, "export BAR=bar")?;
        fs::write(&baz_profile_d_path, "export BAZ=baz")?;
        fs::write(&far_profile_d_path, "export FAR=far")?;

        let result = walk_layers_dir(&mut env, &app_dir, &layers_dir, &buildpacks);
        assert!(result.is_ok());
        if let Ok(profile_d_scripts) = result {
            assert_eq!(
                profile_d_scripts,
                [
                    baz_profile_d_path,
                    bar_profile_d_path,
                    foo_profile_d_path,
                    far_profile_d_path
                ]
                .to_vec()
            );
        };

        let mut path = tools_layer_path.join("bin").as_os_str().to_os_string();
        path.push(":");
        path.push(ruby_layer_path.join("bin").as_os_str().to_os_string());
        path.push(":");
        path.push("vendor/ruby/bin");
        assert_eq!(env.var_os("PATH").unwrap(), path);
        assert_eq!(
            env.var_os("LD_LIBRARY_PATH").unwrap().to_str().unwrap(),
            format!(
                "{}:{}",
                tools_layer_path.join("lib").to_str().unwrap(),
                ruby_layer_path.join("lib").to_str().unwrap(),
            ),
        );
        assert_eq!(env.var_os("FOO").unwrap(), "foo");

        Ok(())
    }
}
