use std::collections::HashMap;
use std::fmt;

use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, PartialEq, Eq, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Config {
    #[serde(default)]
    pub interface: String,
    #[serde(default)]
    pub sets: ConfigSets,
}

#[derive(Clone, Debug, PartialEq, Eq, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ConfigSets {
    #[serde(default)]
    pub all: String,
    #[serde(default)]
    pub ip_matrix: String,
    #[serde(default)]
    pub port_matrix: String,
}

#[derive(Clone, Debug, PartialEq, Eq, Default)]
pub struct DB {
    pub users: HashMap<String, UserEntry>,
    pub vms: HashMap<String, String>,
    pub resources: HashMap<String, ResourceEntry>,
    pub access: HashMap<String, Vec<String>>,
}

#[derive(Clone, Debug, PartialEq, Eq, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct UserEntry {
    #[serde(default)]
    pub ip: String,
    #[serde(rename = "pub")]
    #[serde(default)]
    pub pub_key: String,
    #[serde(default)]
    pub comment: String,
    #[serde(default)]
    pub inactive: bool,
}

#[derive(Clone, Debug, PartialEq, Eq, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ResourceEntry {
    #[serde(default)]
    pub vm: String,
    #[serde(default)]
    pub ports: ResourcePorts,
    #[serde(default)]
    pub comment: String,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize)]
pub struct ResourcePort {
    pub protocol: ResourceProtocol,
    pub port: u16,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, Serialize)]
#[serde(rename_all = "lowercase")]
pub enum ResourceProtocol {
    Tcp,
    Udp,
}

#[derive(Clone, Debug, PartialEq, Eq, Default, Serialize)]
pub struct ResourcePorts(pub Vec<ResourcePort>);

impl fmt::Display for ResourceProtocol {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Tcp => f.write_str("tcp"),
            Self::Udp => f.write_str("udp"),
        }
    }
}

impl fmt::Display for ResourcePort {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}:{}", self.protocol, self.port)
    }
}
