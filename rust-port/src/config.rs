use std::fs;
use std::path::Path;

use crate::model::Config;
use crate::utils::reject_yaml_tags;

pub fn load_config(dir: impl AsRef<Path>) -> Result<Config, String> {
    let path = dir.as_ref().join("config.yaml");
    let data = fs::read_to_string(&path).map_err(|err| format!("read config.yaml: {err}"))?;
    let value: serde_yaml::Value =
        serde_yaml::from_str(&data).map_err(|err| format!("parse config.yaml: {err}"))?;
    reject_yaml_tags(&value, "config.yaml")?;
    let cfg: Config =
        serde_yaml::from_value(value).map_err(|err| format!("parse config.yaml: {err}"))?;
    validate_config(&cfg)?;
    Ok(cfg)
}

pub fn validate_config(cfg: &Config) -> Result<(), String> {
    if cfg.interface.is_empty() {
        return Err("config.yaml: interface is required".to_string());
    }
    if cfg.sets.all.is_empty() {
        return Err("config.yaml: sets.all is required".to_string());
    }
    if cfg.sets.ip_matrix.is_empty() {
        return Err("config.yaml: sets.ip_matrix is required".to_string());
    }
    if cfg.sets.port_matrix.is_empty() {
        return Err("config.yaml: sets.port_matrix is required".to_string());
    }
    Ok(())
}
