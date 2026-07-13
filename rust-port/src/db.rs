use std::collections::{HashMap, HashSet};
use std::fs::{self, File};
use std::io::Write;
use std::net::Ipv4Addr;
use std::path::Path;
use std::str::FromStr;

use serde::de::{self, SeqAccess, Visitor};
use serde::{Deserialize, Deserializer};
use tempfile::Builder;

use crate::model::{ResourceEntry, ResourcePort, ResourcePorts, ResourceProtocol, UserEntry, DB};
use crate::utils::{case_fold_ascii, reject_yaml_tags, sorted_keys};

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct RawDB {
    users: Option<HashMap<String, UserEntry>>,
    vms: Option<HashMap<String, String>>,
    #[serde(default)]
    resources: HashMap<String, ResourceEntry>,
    #[serde(default)]
    access: HashMap<String, Vec<String>>,
}

pub fn load_db(dir: impl AsRef<Path>) -> Result<DB, String> {
    let path = dir.as_ref().join("db.yaml");
    let data = fs::read_to_string(&path).map_err(|err| format!("read db.yaml: {err}"))?;
    let value: serde_yaml::Value =
        serde_yaml::from_str(&data).map_err(|err| format!("parse db.yaml: {err}"))?;
    reject_yaml_tags(&value, "db.yaml")?;
    let raw: RawDB =
        serde_yaml::from_value(value).map_err(|err| format!("parse db.yaml: {err}"))?;
    let has_users = raw.users.is_some();
    let has_vms = raw.vms.is_some();
    let db = DB {
        users: raw.users.unwrap_or_default(),
        vms: raw.vms.unwrap_or_default(),
        resources: raw.resources,
        access: raw.access,
    };
    let mut errs = validate_db_with_required(&db, has_users, has_vms);
    if errs.is_empty() {
        Ok(db)
    } else {
        errs.sort();
        Err(errs.join("; "))
    }
}

impl<'de> Deserialize<'de> for ResourcePorts {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserializer.deserialize_any(ResourcePortsVisitor)
    }
}

struct ResourcePortsVisitor;

impl<'de> Visitor<'de> for ResourcePortsVisitor {
    type Value = ResourcePorts;

    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("a resource port scalar or sequence")
    }

    fn visit_str<E>(self, value: &str) -> Result<Self::Value, E>
    where
        E: de::Error,
    {
        parse_resource_port_scalar(value)
            .map(|port| ResourcePorts(vec![port]))
            .map_err(E::custom)
    }

    fn visit_u64<E>(self, value: u64) -> Result<Self::Value, E>
    where
        E: de::Error,
    {
        parse_resource_port_scalar(&value.to_string())
            .map(|port| ResourcePorts(vec![port]))
            .map_err(E::custom)
    }

    fn visit_i64<E>(self, value: i64) -> Result<Self::Value, E>
    where
        E: de::Error,
    {
        parse_resource_port_scalar(&value.to_string())
            .map(|port| ResourcePorts(vec![port]))
            .map_err(E::custom)
    }

    fn visit_seq<A>(self, mut seq: A) -> Result<Self::Value, A::Error>
    where
        A: SeqAccess<'de>,
    {
        let mut out = Vec::new();
        while let Some(item) = seq.next_element::<ResourcePortScalar>()? {
            out.push(item.0);
        }
        Ok(ResourcePorts(out))
    }
}

struct ResourcePortScalar(ResourcePort);

impl<'de> Deserialize<'de> for ResourcePortScalar {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        deserializer.deserialize_any(ResourcePortScalarVisitor)
    }
}

struct ResourcePortScalarVisitor;

impl<'de> Visitor<'de> for ResourcePortScalarVisitor {
    type Value = ResourcePortScalar;

    fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("a resource port scalar")
    }

    fn visit_str<E>(self, value: &str) -> Result<Self::Value, E>
    where
        E: de::Error,
    {
        parse_resource_port_scalar(value)
            .map(ResourcePortScalar)
            .map_err(E::custom)
    }

    fn visit_u64<E>(self, value: u64) -> Result<Self::Value, E>
    where
        E: de::Error,
    {
        parse_resource_port_scalar(&value.to_string())
            .map(ResourcePortScalar)
            .map_err(E::custom)
    }

    fn visit_i64<E>(self, value: i64) -> Result<Self::Value, E>
    where
        E: de::Error,
    {
        parse_resource_port_scalar(&value.to_string())
            .map(ResourcePortScalar)
            .map_err(E::custom)
    }
}

pub fn parse_resource_port_scalar(value: &str) -> Result<ResourcePort, String> {
    let value = value.trim();
    if value.is_empty() {
        return Err("resource port must not be empty".to_string());
    }

    let (protocol, port_text) = match value.split_once(':') {
        Some((protocol, port_text)) => (protocol.trim().to_ascii_lowercase(), port_text.trim()),
        None => ("tcp".to_string(), value),
    };

    let protocol = match protocol.as_str() {
        "tcp" => ResourceProtocol::Tcp,
        "udp" => ResourceProtocol::Udp,
        _ => {
            return Err(format!(
                "resource port protocol {protocol:?} must be tcp or udp"
            ))
        }
    };

    let port: u32 = port_text
        .parse()
        .map_err(|_| format!("resource port {port_text:?} is not numeric"))?;
    if !(1..=65535).contains(&port) {
        return Err(format!("resource port {port} is outside 1..65535"));
    }

    Ok(ResourcePort {
        protocol,
        port: port as u16,
    })
}

pub fn validate_db(db: &DB) -> Vec<String> {
    validate_db_with_required(db, true, true)
}

fn validate_db_with_required(db: &DB, has_users: bool, has_vms: bool) -> Vec<String> {
    let mut errs = Vec::new();

    if !has_users {
        errs.push("db.yaml: users section is required".to_string());
    }
    if !has_vms {
        errs.push("db.yaml: vms section is required".to_string());
    }

    let mut seen_user_lower: HashMap<String, String> = HashMap::new();
    for (name, user) in &db.users {
        if !valid_user_or_vm_name(name) {
            errs.push(format!("db.yaml: invalid user name {name:?}"));
        }
        let folded = case_fold_ascii(name);
        if let Some(conflict) = seen_user_lower.insert(folded, name.clone()) {
            errs.push(format!(
                "db.yaml: user name {name:?} conflicts with {conflict:?} (case)"
            ));
        }
        if user.ip.is_empty() {
            errs.push(format!("db.yaml: user {name:?}: ip is required"));
        } else if !is_ipv4(&user.ip) {
            errs.push(format!(
                "db.yaml: user {name:?} has invalid ip {:?}",
                user.ip
            ));
        }
        if user.pub_key.is_empty() {
            errs.push(format!("db.yaml: user {name:?}: pub is required"));
        }
    }

    let mut seen_vm_lower: HashMap<String, String> = HashMap::new();
    for (name, ip_value) in &db.vms {
        if !valid_user_or_vm_name(name) {
            errs.push(format!("db.yaml: invalid vm name {name:?}"));
        }
        let folded = case_fold_ascii(name);
        if let Some(conflict) = seen_vm_lower.insert(folded, name.clone()) {
            errs.push(format!(
                "db.yaml: vm name {name:?} conflicts with {conflict:?} (case)"
            ));
        }
        if !is_ipv4(ip_value) {
            errs.push(format!("db.yaml: vm {name:?} has invalid ip {ip_value:?}"));
        }
    }

    let mut seen_target_lower: HashMap<String, String> = HashMap::new();
    for name in db.vms.keys() {
        seen_target_lower.insert(case_fold_ascii(name), name.clone());
    }
    for (name, resource) in &db.resources {
        if !valid_resource_name(name) {
            errs.push(format!("db.yaml: invalid resource name {name:?}"));
        }
        if name == "*" {
            errs.push("db.yaml: resource name \"*\" is reserved".to_string());
        }
        if db.vms.contains_key(name) {
            errs.push(format!(
                "db.yaml: resource {name:?} conflicts with VM of the same name"
            ));
        }
        let folded = case_fold_ascii(name);
        if let Some(conflict) = seen_target_lower.insert(folded, name.clone()) {
            errs.push(format!(
                "db.yaml: resource name {name:?} conflicts with {conflict:?} (case)"
            ));
        }
        if resource.vm.is_empty() {
            errs.push(format!("db.yaml: resource {name:?}: vm is required"));
        } else if !db.vms.contains_key(&resource.vm) {
            errs.push(format!(
                "db.yaml: resource {name:?} references unknown vm {:?}",
                resource.vm
            ));
        }
        if resource.ports.0.is_empty() {
            errs.push(format!("db.yaml: resource {name:?}: ports is required"));
        }
        let mut seen_ports = HashSet::new();
        for port in &resource.ports.0 {
            let key = port.to_string();
            if !seen_ports.insert(key.clone()) {
                errs.push(format!(
                    "db.yaml: resource {name:?} contains duplicate port {key}"
                ));
            }
        }
    }

    let mut seen_ips: HashMap<&str, &str> = HashMap::new();
    for (name, user) in &db.users {
        if user.ip.is_empty() {
            continue;
        }
        if let Some(prev) = seen_ips.insert(&user.ip, name) {
            errs.push(format!(
                "db.yaml: duplicate ip {} for users {prev:?} and {name:?}",
                user.ip
            ));
        }
    }

    let mut seen_pubs: HashMap<&str, &str> = HashMap::new();
    for (name, user) in &db.users {
        if user.pub_key.is_empty() {
            continue;
        }
        if let Some(prev) = seen_pubs.insert(&user.pub_key, name) {
            errs.push(format!(
                "db.yaml: duplicate pub key for users {prev:?} and {name:?}"
            ));
        }
    }

    for (user, entries) in &db.access {
        if !db.users.contains_key(user) {
            errs.push(format!("db.yaml: access references unknown user {user:?}"));
        }
        let mut has_admin_star = false;
        let mut seen_access: HashSet<&str> = HashSet::new();
        for entry in entries {
            if !seen_access.insert(entry) {
                errs.push(format!(
                    "db.yaml: access for user {user:?} contains duplicate entry {entry:?}"
                ));
            }
            if entry == "*" {
                has_admin_star = true;
                continue;
            }
            if !valid_resource_name(entry) {
                errs.push(format!(
                    "db.yaml: access for user {user:?}: invalid access target name {entry:?}"
                ));
            }
            if !db.vms.contains_key(entry) && !db.resources.contains_key(entry) {
                errs.push(format!(
                    "db.yaml: access for user {user:?} references unknown access target {entry:?}"
                ));
            }
        }
        if has_admin_star && entries.len() > 1 {
            errs.push(format!(
                "db.yaml: user {user:?}: access contains \"*\" mixed with other VMs; \"*\" must be the sole entry"
            ));
        }
    }

    errs.sort();
    errs
}

pub fn clone_db(db: &DB) -> DB {
    db.clone()
}

pub fn normalize_db_access(db: &mut DB) {
    db.access.retain(|_, entries| {
        if entries.is_empty() {
            return false;
        }
        let set: HashSet<String> = entries.iter().cloned().collect();
        let normalized = normalize_access_set(&set);
        if normalized.is_empty() {
            false
        } else {
            *entries = normalized;
            true
        }
    });
}

pub fn normalize_access_set(set: &HashSet<String>) -> Vec<String> {
    let mut entries: Vec<String> = set.iter().cloned().collect();
    entries.sort();
    entries
}

pub fn save_db_atomic(dir: impl AsRef<Path>, db: &DB) -> Result<(), String> {
    let dir = dir.as_ref();
    let data = marshal_db_deterministic(db);
    let mut tmp = Builder::new()
        .prefix(".db.yaml.")
        .tempfile_in(dir)
        .map_err(|err| format!("create temporary db.yaml: {err}"))?;
    tmp.write_all(data.as_bytes())
        .map_err(|err| format!("write temporary db.yaml: {err}"))?;
    set_file_mode_0600(tmp.path()).map_err(|err| format!("chmod temporary db.yaml: {err}"))?;
    tmp.as_file_mut()
        .sync_all()
        .map_err(|err| format!("sync temporary db.yaml: {err}"))?;

    let temp_path = tmp.into_temp_path();
    let final_path = dir.join("db.yaml");
    fs::rename(&temp_path, &final_path).map_err(|err| format!("replace db.yaml: {err}"))?;
    sync_dir(dir)?;
    Ok(())
}

pub fn marshal_db_deterministic(db: &DB) -> String {
    let mut out = String::new();
    write_users(&mut out, &db.users);
    out.push('\n');
    write_vms(&mut out, &db.vms);
    out.push('\n');
    write_resources(&mut out, &db.resources);
    out.push('\n');
    write_access(&mut out, &db.access);
    out
}

fn write_users(out: &mut String, users: &HashMap<String, UserEntry>) {
    if users.is_empty() {
        out.push_str("users: {}\n");
        return;
    }
    out.push_str("users:\n");
    for name in sorted_keys(users) {
        let user = &users[&name];
        out.push_str("  ");
        out.push_str(&yaml_key(&name));
        out.push_str(":\n");
        out.push_str("    ip: ");
        out.push_str(&yaml_scalar(&user.ip));
        out.push('\n');
        out.push_str("    pub: ");
        out.push_str(&yaml_scalar(&user.pub_key));
        out.push('\n');
        if !user.comment.is_empty() {
            out.push_str("    comment: ");
            out.push_str(&yaml_scalar(&user.comment));
            out.push('\n');
        }
        if user.inactive {
            out.push_str("    inactive: true\n");
        }
    }
}

fn write_vms(out: &mut String, vms: &HashMap<String, String>) {
    if vms.is_empty() {
        out.push_str("vms: {}\n");
        return;
    }
    out.push_str("vms:\n");
    for name in sorted_keys(vms) {
        out.push_str("  ");
        out.push_str(&yaml_key(&name));
        out.push_str(": ");
        out.push_str(&yaml_scalar(&vms[&name]));
        out.push('\n');
    }
}

fn write_resources(out: &mut String, resources: &HashMap<String, ResourceEntry>) {
    if resources.is_empty() {
        out.push_str("resources: {}\n");
        return;
    }
    out.push_str("resources:\n");
    for name in sorted_keys(resources) {
        let resource = &resources[&name];
        out.push_str("  ");
        out.push_str(&yaml_key(&name));
        out.push_str(":\n");
        out.push_str("    vm: ");
        out.push_str(&yaml_scalar(&resource.vm));
        out.push('\n');
        match resource.ports.0.as_slice() {
            [port] => {
                out.push_str("    ports: ");
                out.push_str(&yaml_scalar(&port.to_string()));
                out.push('\n');
            }
            ports => {
                out.push_str("    ports:\n");
                for port in ports {
                    out.push_str("      - ");
                    out.push_str(&yaml_scalar(&port.to_string()));
                    out.push('\n');
                }
            }
        }
        if !resource.comment.is_empty() {
            out.push_str("    comment: ");
            out.push_str(&yaml_scalar(&resource.comment));
            out.push('\n');
        }
    }
}

fn write_access(out: &mut String, access: &HashMap<String, Vec<String>>) {
    let keys: Vec<String> = sorted_keys(access)
        .into_iter()
        .filter(|key| !access[key].is_empty())
        .collect();
    if keys.is_empty() {
        out.push_str("access: {}\n");
        return;
    }
    out.push_str("access:\n");
    for key in keys {
        out.push_str("  ");
        out.push_str(&yaml_key(&key));
        out.push_str(":\n");
        for entry in &access[&key] {
            out.push_str("    - ");
            out.push_str(&yaml_scalar(entry));
            out.push('\n');
        }
    }
}

fn valid_user_or_vm_name(value: &str) -> bool {
    !value.is_empty()
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
}

fn valid_resource_name(value: &str) -> bool {
    !value.is_empty()
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'@' || b == b'-')
}

fn is_ipv4(value: &str) -> bool {
    Ipv4Addr::from_str(value).is_ok()
}

fn yaml_key(value: &str) -> String {
    yaml_scalar(value)
}

fn yaml_scalar(value: &str) -> String {
    if can_write_plain_scalar(value) {
        return value.to_string();
    }
    let mut escaped = String::with_capacity(value.len() + 2);
    escaped.push('"');
    for ch in value.chars() {
        match ch {
            '\\' => escaped.push_str("\\\\"),
            '"' => escaped.push_str("\\\""),
            '\n' => escaped.push_str("\\n"),
            '\r' => escaped.push_str("\\r"),
            '\t' => escaped.push_str("\\t"),
            _ => escaped.push(ch),
        }
    }
    escaped.push('"');
    escaped
}

fn can_write_plain_scalar(value: &str) -> bool {
    if value.is_empty() || value == "*" {
        return false;
    }
    if value.starts_with([
        '-', '?', ':', '!', '&', '*', '#', '{', '}', '[', ']', ',', '"', '\'',
    ]) {
        return false;
    }
    if matches!(value, "true" | "false" | "null" | "~") {
        return false;
    }
    let mut chars = value.chars().peekable();
    let mut prev = None;
    while let Some(ch) = chars.next() {
        if ch.is_control() {
            return false;
        }
        if ch == '#' && prev == Some(' ') {
            return false;
        }
        if ch == ':' && matches!(chars.peek(), None | Some(' ')) {
            return false;
        }
        prev = Some(ch);
    }
    if value.ends_with(' ') {
        return false;
    }

    matches!(
        serde_yaml::from_str::<serde_yaml::Value>(value),
        Ok(serde_yaml::Value::String(parsed)) if parsed == value
    )
}

fn set_file_mode_0600(path: &Path) -> std::io::Result<()> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o600))
    }
    #[cfg(not(unix))]
    {
        let _ = path;
        Ok(())
    }
}

fn sync_dir(dir: &Path) -> Result<(), String> {
    #[cfg(unix)]
    {
        File::open(dir)
            .and_then(|file| file.sync_all())
            .map_err(|err| format!("sync config directory: {err}"))?;
    }
    #[cfg(not(unix))]
    {
        let _ = dir;
    }
    Ok(())
}
