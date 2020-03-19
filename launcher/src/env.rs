use failure::{format_err, Error};
use std::{
    ffi::{CString, OsStr, OsString},
    path::Path,
};

pub enum Position {
    Prefix,
    Suffix,
}

pub trait Env {
    fn vars_os(&self) -> Box<dyn Iterator<Item = (OsString, OsString)>>;
    fn set_var<K: AsRef<OsStr>, V: AsRef<OsStr>>(&mut self, k: K, v: V);
    fn var_os<K: AsRef<OsStr>>(&self, k: K) -> Option<OsString>;
    fn modify_var<K: AsRef<OsStr>, P: AsRef<Path>>(
        &mut self,
        k: K,
        position: Position,
        v: P,
    ) -> Result<(), Error> {
        let value = v.as_ref();
        let env_value = &self.var_os(&k);

        if let Some(env_value) = env_value {
            let mut paths = std::env::split_paths(env_value).collect::<Vec<_>>();

            match position {
                Position::Prefix => paths.insert(0, value.to_path_buf()),
                Position::Suffix => paths.push(value.to_path_buf()),
            }
            self.set_var(&k, std::env::join_paths(paths)?);
        } else {
            self.set_var(&k, value);
        }

        Ok(())
    }
    fn list(&self) -> Result<Vec<CString>, Error> {
        self.vars_os()
            .map(|(key, value)| {
                let key_string = key
                    .to_str()
                    .ok_or_else(|| format_err!("env var key '{:?}' is not valid unicode", key))?;
                let value_string = value
                    .to_str()
                    .ok_or_else(|| format_err!("env var '{:?}' is not valid unicode", value))?;
                Ok(CString::new(format!("{}={}", key_string, value_string))?)
            })
            .collect()
    }
}

pub struct OsEnv;

impl Env for OsEnv {
    fn vars_os(&self) -> Box<dyn Iterator<Item = (OsString, OsString)>> {
        Box::new(std::env::vars_os())
    }

    fn set_var<K: AsRef<OsStr>, V: AsRef<OsStr>>(&mut self, k: K, v: V) {
        std::env::set_var(k, v)
    }

    fn var_os<K: AsRef<OsStr>>(&self, k: K) -> Option<OsString> {
        std::env::var_os(k)
    }
}

#[cfg(test)]
pub mod test_helpers {
    use super::Env;
    use std::{
        collections::HashMap,
        ffi::{OsStr, OsString},
    };

    pub struct TestEnv {
        pub inner: HashMap<OsString, OsString>,
    }

    impl TestEnv {
        pub fn new() -> Self {
            Self {
                inner: HashMap::new(),
            }
        }
    }

    impl Env for TestEnv {
        fn vars_os(&self) -> Box<dyn Iterator<Item = (OsString, OsString)>> {
            Box::new(self.inner.clone().into_iter())
        }

        fn set_var<K: AsRef<OsStr>, V: AsRef<OsStr>>(&mut self, k: K, v: V) {
            let key = k.as_ref();
            let value = v.as_ref();

            self.inner.insert(key.to_os_string(), value.to_os_string());
        }

        fn var_os<K: AsRef<OsStr>>(&self, k: K) -> Option<OsString> {
            let key = k.as_ref();

            self.inner.get(key).cloned()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::test_helpers::TestEnv;
    use super::*;

    #[test]
    fn it_modifies_var_path_prefix_empty() {
        let mut env = TestEnv::new();
        let value = Path::new("/tmp/foo");
        let key = "TEST";

        assert!(env.modify_var(key, Position::Prefix, &value).is_ok());
        assert_eq!(env.var_os(key).unwrap(), value);
    }

    #[test]
    fn it_modifies_var_path_suffix_empty() {
        let mut env = TestEnv::new();
        let value = Path::new("/tmp/foo");
        let key = "TEST";

        assert!(env.modify_var(key, Position::Suffix, &value).is_ok());
        assert_eq!(env.var_os(key).unwrap(), value);
    }

    #[test]
    fn it_modifies_var_path_prefix() {
        let mut env = TestEnv::new();
        let value = Path::new("/tmp/foo");
        let key = "TEST";
        env.set_var(key, "/tmp/bar");

        assert!(env.modify_var(key, Position::Prefix, &value).is_ok());
        assert_eq!(env.var_os(key).unwrap(), "/tmp/foo:/tmp/bar");
    }

    #[test]
    fn it_modifies_var_path_suffix() {
        let mut env = TestEnv::new();
        let value = Path::new("/tmp/foo");
        let key = "TEST";
        env.set_var(key, "/tmp/bar");

        assert!(env.modify_var(key, Position::Suffix, &value).is_ok());
        assert_eq!(env.var_os(key).unwrap(), "/tmp/bar:/tmp/foo");
    }

    #[test]
    fn it_returns_key_value_list() {
        let mut env = TestEnv::new();
        env.set_var("FOO", "/tmp/foo");
        env.set_var("BAR", Path::new("/tmp/bar"));

        let envs = env.list();
        assert!(envs.is_ok());
        if let Ok(envs) = envs {
            assert_eq!(envs.len(), 2);
            assert!(envs.contains(&CString::new("BAR=/tmp/bar").unwrap()));
            assert!(envs.contains(&CString::new("FOO=/tmp/foo").unwrap()));
        }
    }
}
