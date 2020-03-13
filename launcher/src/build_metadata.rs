use serde::Deserialize;

#[derive(Clone, Debug, Deserialize)]
pub struct BOMEntry {
    pub name: String,
    pub version: String,
    pub buildpack: Buildpack,
    pub metadata: toml::map::Map<String, toml::Value>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct Buildpack {
    pub id: String,
    pub version: String,
}

impl Buildpack {
    #[allow(dead_code)]
    pub fn new<I: Into<String>, V: Into<String>>(i: I, v: V) -> Self {
        Self {
            id: i.into(),
            version: v.into(),
        }
    }

    pub fn path_id(&self) -> String {
        self.id.replace(std::path::MAIN_SEPARATOR, "_")
    }
}

#[derive(Clone, Debug, Deserialize, PartialEq)]
pub struct Process {
    pub r#type: String,
    pub command: String,
    #[serde(default)]
    pub args: Vec<String>,
    #[serde(default)]
    pub direct: bool,
}

#[derive(Clone, Debug, Deserialize)]
pub struct BuildMetadata {
    pub processes: Vec<Process>,
    pub buildpacks: Vec<Buildpack>,
    pub bom: Option<Vec<BOMEntry>>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use failure::Error;

    #[test]
    fn it_replaces_separator_in_path_id() {
        let buildpack = Buildpack::new("heroku/ruby", "1.0");

        assert_eq!("heroku_ruby", buildpack.path_id());
    }

    #[test]
    fn it_parses_bom() -> Result<(), Error> {
        let input = r#"
name = "jdk"
version = "1.8.0_232"
[metadata]
  launch = false
  vendor = "OpenJDK"
[buildpack]
  id = "heroku/jvm"
  version = "0.1"
"#;

        let result: Result<BOMEntry, toml::de::Error> = toml::from_str(input);
        assert!(result.is_ok());

        Ok(())
    }

    #[test]
    fn it_parses_buildpack() -> Result<(), Error> {
        let input = r#"
id = "heroku/jvm"
version = "0.1"
"#;

        let result: Result<Buildpack, toml::de::Error> = toml::from_str(input);
        assert!(result.is_ok());

        Ok(())
    }

    #[test]
    fn it_parses_process() -> Result<(), Error> {
        let input = r#"
type = "web"
command = "java -jar target/java-getting-started-1.0.jar"
"#;

        let result: Result<Process, toml::de::Error> = toml::from_str(input);
        assert!(result.is_ok());

        if let Ok(process) = result {
            assert!(process.args.is_empty());
            assert_eq!(process.direct, false);
        }

        Ok(())
    }

    #[test]
    fn it_parses_process_with_optional_args() -> Result<(), Error> {
        let input = r#"
type = "web"
command = "java"
args = ["-jar", "target/java-getting-started-1.0.jar"]
direct = true
"#;

        let result: Result<Process, toml::de::Error> = toml::from_str(input);
        assert!(result.is_ok());

        Ok(())
    }

    #[test]
    fn it_parses_build_metadata() -> Result<(), Error> {
        let input = r#"
[[processes]]
  type = "web"
  command = "java -jar target/java-getting-started-1.0.jar"
  direct = false

[[buildpacks]]
  id = "heroku/jvm"
  version = "0.1"

[[buildpacks]]
  id = "heroku/maven"
  version = "0.1"

[[buildpacks]]
  id = "heroku/procfile"
  version = "0.3"

[[bom]]
  name = "jdk"
  version = "1.8.0_232"
  [bom.metadata]
    launch = false
    vendor = "OpenJDK"
  [bom.buildpack]
    id = "heroku/jvm"
    version = "0.1"

[[bom]]
  name = "jre"
  version = "1.8.0_232"
  [bom.metadata]
    launch = true
    vendor = "OpenJDK"
  [bom.buildpack]
    id = "heroku/jvm"
    version = "0.1"
"#;

        let result: Result<BuildMetadata, toml::de::Error> = toml::from_str(input);
        assert!(result.is_ok());

        Ok(())
    }
}
