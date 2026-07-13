use std::collections::HashMap;
use std::fmt;

use serde::{Deserialize, Serialize};

use crate::parse_wg::WGDumpResult;

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

#[derive(Clone, Debug, PartialEq, Eq, Default)]
pub struct ExpectedIPSets {
    pub all: HashMap<String, String>,
    pub ip_matrix: HashMap<String, String>,
    pub port_matrix: HashMap<String, String>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct IPSetDelta {
    pub set: String,
    pub entry: String,
    pub comment: String,
    pub add: bool,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub enum WGPeerDeltaAction {
    Add,
    Remove,
}

impl fmt::Display for WGPeerDeltaAction {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Add => f.write_str("add"),
            Self::Remove => f.write_str("remove"),
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct WGPeerDelta {
    pub user: String,
    pub pub_key: String,
    pub allowed_ip: String,
    pub action: WGPeerDeltaAction,
}

#[derive(Clone, Debug, PartialEq, Eq, Default)]
pub struct CheckResult {
    pub hard_errors: Vec<String>,
    pub drift: Vec<String>,
    pub ipset_deltas: Vec<IPSetDelta>,
    pub peer_deltas: Vec<WGPeerDelta>,
    pub wg_dump: Option<WGDumpResult>,
}

impl CheckResult {
    pub fn ok(&self) -> bool {
        self.hard_errors.is_empty() && self.drift.is_empty() && self.peer_deltas.is_empty()
    }

    pub fn clean(&self) -> bool {
        self.hard_errors.is_empty()
    }
}
