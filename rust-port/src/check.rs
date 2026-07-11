use std::collections::HashMap;
use std::net::Ipv4Addr;

use crate::config::validate_config;
use crate::db::{parse_resource_port_scalar, validate_db};
use crate::model::{
    CheckResult, Config, ExpectedIPSets, IPSetDelta, WGPeerDelta, WGPeerDeltaAction, DB,
};
use crate::parse_ipset::{parse_ipset, IPSetEntry, ParsedIPSet};
use crate::parse_wg::parse_wg_dump;
use crate::system::SystemAdapter;
use crate::utils::{is_valid_ipv4_or_cidr, parse_ipv4_cidr};

pub fn check<S: SystemAdapter>(cfg: &Config, db: &DB, system: &S) -> CheckResult {
    let mut result = CheckResult::default();

    if let Err(err) = validate_config(cfg) {
        result.hard_errors.push(err);
    }
    result.hard_errors.extend(validate_db(db));
    if !result.clean() {
        result.hard_errors.sort();
        return result;
    }

    let subnet = match system.interface_subnet(&cfg.interface) {
        Ok(subnet) => subnet,
        Err(err) => {
            result.hard_errors.push(format!(
                "get interface subnet for {:?}: {err}",
                cfg.interface
            ));
            return result;
        }
    };
    let parsed_subnet = match parse_ipv4_cidr(&subnet) {
        Ok(subnet) => subnet,
        Err(err) => {
            result
                .hard_errors
                .push(format!("parse interface subnet {subnet:?}: {err}"));
            return result;
        }
    };

    for (name, user) in &db.users {
        let Ok(ip) = user.ip.parse::<Ipv4Addr>() else {
            continue;
        };
        if !ipv4_cidr_contains(&parsed_subnet, ip) {
            result.hard_errors.push(format!(
                "user {name:?} ip {} is outside interface subnet {subnet}",
                user.ip
            ));
        }
    }

    let wg_raw = match system.wg_dump(&cfg.interface) {
        Ok(raw) => raw,
        Err(err) => {
            result
                .hard_errors
                .push(format!("wg dump {:?}: {err}", cfg.interface));
            sort_result(&mut result);
            return result;
        }
    };
    let wg_dump = match parse_wg_dump(&wg_raw) {
        Ok(dump) => dump,
        Err(err) => {
            result.hard_errors.push(format!("parse wg dump: {err}"));
            sort_result(&mut result);
            return result;
        }
    };

    let mut wg_by_pub = HashMap::new();
    for peer in &wg_dump.peers {
        wg_by_pub.insert(peer.public_key.as_str(), peer);
    }
    let mut db_by_pub = HashMap::new();
    for (name, user) in &db.users {
        db_by_pub.insert(user.pub_key.as_str(), name.as_str());
    }
    result.wg_dump = Some(wg_dump.clone());

    for (name, user) in &db.users {
        let peer = wg_by_pub.get(user.pub_key.as_str());
        if user.inactive {
            if peer.is_some() {
                result.peer_deltas.push(WGPeerDelta {
                    user: name.clone(),
                    pub_key: user.pub_key.clone(),
                    allowed_ip: user.ip.clone(),
                    action: WGPeerDeltaAction::Remove,
                });
            }
            continue;
        }

        match peer {
            Some(peer) if peer.allowed_ip != user.ip => {
                result.hard_errors.push(format!(
                    "user {name:?}: db ip {} does not match WireGuard allowed-ip {}",
                    user.ip, peer.allowed_ip
                ));
            }
            Some(_) => {}
            None => result.peer_deltas.push(WGPeerDelta {
                user: name.clone(),
                pub_key: user.pub_key.clone(),
                allowed_ip: user.ip.clone(),
                action: WGPeerDeltaAction::Add,
            }),
        }
    }

    if let Some(wg_dump) = &result.wg_dump {
        for peer in &wg_dump.peers {
            if !db_by_pub.contains_key(peer.public_key.as_str()) {
                result.hard_errors.push(format!(
                    "WireGuard peer {} not found in db users",
                    peer.public_key
                ));
            }
        }
    }

    if !result.clean() {
        sort_result(&mut result);
        return result;
    }

    let expected = compute_expected_ipsets(db);
    check_ipset(
        &cfg.sets.all,
        "hash:ip",
        &expected.all,
        &mut result,
        validate_all_access_ipset,
        system,
    );
    check_ipset(
        &cfg.sets.ip_matrix,
        "hash:net,net",
        &expected.ip_matrix,
        &mut result,
        validate_ip_matrix_ipset,
        system,
    );
    check_ipset(
        &cfg.sets.port_matrix,
        "hash:ip,port,ip",
        &expected.port_matrix,
        &mut result,
        validate_port_matrix_ipset,
        system,
    );

    sort_result(&mut result);
    result
}

pub fn compute_expected_ipsets(db: &DB) -> ExpectedIPSets {
    let mut expected = ExpectedIPSets::default();

    for (user_name, targets) in &db.access {
        let Some(user) = db.users.get(user_name) else {
            continue;
        };
        if user.inactive {
            continue;
        }

        for target in targets {
            if target == "*" {
                expected.all.insert(user.ip.clone(), String::new());
                continue;
            }

            if let Some(vm_ip) = db.vms.get(target) {
                expected.ip_matrix.insert(
                    format!("{},{}", user.ip, vm_ip),
                    format!("{user_name} -> {target}"),
                );
                continue;
            }

            let Some(resource) = db.resources.get(target) else {
                continue;
            };
            let Some(vm_ip) = db.vms.get(&resource.vm) else {
                continue;
            };
            for port in &resource.ports.0 {
                expected.port_matrix.insert(
                    format!("{},{},{}", user.ip, port, vm_ip),
                    format!("{user_name} -> {target} {}/{}", port.protocol, port.port),
                );
            }
        }
    }

    expected
}

fn check_ipset<S, F>(
    set_name: &str,
    expected_type: &str,
    expected: &HashMap<String, String>,
    result: &mut CheckResult,
    validate: F,
    system: &S,
) where
    S: SystemAdapter,
    F: Fn(&str, &ParsedIPSet) -> Vec<String>,
{
    let raw = match system.ipset_list(set_name) {
        Ok(raw) => raw,
        Err(err) => {
            result.hard_errors.push(format!(
                "ipset list {set_name:?}: {err} (run 'wgman init-ipsets' to create managed sets)"
            ));
            return;
        }
    };
    let parsed = match parse_ipset(&raw) {
        Ok(parsed) => parsed,
        Err(err) => {
            result
                .hard_errors
                .push(format!("parse ipset {set_name:?}: {err}"));
            return;
        }
    };

    let errors = validate(set_name, &parsed);
    if !errors.is_empty() {
        result.hard_errors.extend(errors);
        return;
    }
    if parsed.set_type != expected_type {
        result.hard_errors.push(format!(
            "ipset {set_name:?} has type {:?}, expected {expected_type} (run 'wgman init-ipsets' to recreate)",
            parsed.set_type
        ));
        return;
    }

    reconcile_ipset(set_name, expected, &parsed.entries, result);
}

fn reconcile_ipset(
    set_name: &str,
    expected: &HashMap<String, String>,
    live: &[IPSetEntry],
    result: &mut CheckResult,
) {
    let live_set: std::collections::HashSet<&str> =
        live.iter().map(|entry| entry.entry.as_str()).collect();

    for (entry, comment) in expected {
        if !live_set.contains(entry.as_str()) {
            result
                .drift
                .push(format!("ipset {set_name}: missing entry {entry}"));
            result.ipset_deltas.push(IPSetDelta {
                set: set_name.to_string(),
                entry: entry.clone(),
                comment: comment.clone(),
                add: true,
            });
        }
    }

    for entry in live {
        if !expected.contains_key(&entry.entry) {
            result.drift.push(format!(
                "ipset {set_name}: unexpected entry {}",
                entry.entry
            ));
            result.ipset_deltas.push(IPSetDelta {
                set: set_name.to_string(),
                entry: entry.entry.clone(),
                comment: String::new(),
                add: false,
            });
        }
    }
}

fn validate_all_access_ipset(set_name: &str, parsed: &ParsedIPSet) -> Vec<String> {
    let mut errors = validate_set_header(set_name, "hash:ip", parsed);
    if !errors.is_empty() {
        return errors;
    }
    for entry in &parsed.entries {
        if entry.entry.parse::<Ipv4Addr>().is_err() {
            errors.push(format!(
                "ipset {set_name:?}: all-access entry {:?} is not a valid IPv4 address",
                entry.entry
            ));
        }
    }
    errors
}

fn validate_ip_matrix_ipset(set_name: &str, parsed: &ParsedIPSet) -> Vec<String> {
    let mut errors = validate_set_header(set_name, "hash:net,net", parsed);
    if !errors.is_empty() {
        return errors;
    }
    for entry in &parsed.entries {
        let parts: Vec<&str> = entry.entry.splitn(2, ',').collect();
        if parts.len() != 2 || !is_valid_ipv4_or_cidr(parts[0]) || !is_valid_ipv4_or_cidr(parts[1])
        {
            errors.push(format!(
                "ipset {set_name:?}: matrix entry {:?} is not two valid IPv4/net values",
                entry.entry
            ));
        }
    }
    errors
}

fn validate_port_matrix_ipset(set_name: &str, parsed: &ParsedIPSet) -> Vec<String> {
    let mut errors = validate_set_header(set_name, "hash:ip,port,ip", parsed);
    if !errors.is_empty() {
        return errors;
    }
    for entry in &parsed.entries {
        let parts: Vec<&str> = entry.entry.split(',').collect();
        if parts.len() != 3 {
            errors.push(format!(
                "ipset {set_name:?}: port matrix entry {:?} is not source-ip,port,destination-ip",
                entry.entry
            ));
            continue;
        }
        if parts[0].parse::<Ipv4Addr>().is_err() || parts[2].parse::<Ipv4Addr>().is_err() {
            errors.push(format!(
                "ipset {set_name:?}: port matrix entry {:?} must use IPv4 source and destination addresses",
                entry.entry
            ));
        }
        if let Err(err) = parse_resource_port_scalar(parts[1]) {
            errors.push(format!(
                "ipset {set_name:?}: port matrix entry {:?} has invalid port: {err}",
                entry.entry
            ));
        }
    }
    errors
}

fn validate_set_header(set_name: &str, expected_type: &str, parsed: &ParsedIPSet) -> Vec<String> {
    let mut errors = Vec::new();
    if parsed.set_name != set_name {
        errors.push(format!(
            "ipset {set_name:?} output describes set {:?}, expected {set_name:?}",
            parsed.set_name
        ));
        return errors;
    }
    if parsed.set_type != expected_type {
        errors.push(format!(
            "ipset {set_name:?} has type {:?}, expected {expected_type} (run 'wgman init-ipsets' to recreate)",
            parsed.set_type
        ));
    }
    errors
}

fn ipv4_cidr_contains(cidr: &crate::utils::IPv4Cidr, ip: Ipv4Addr) -> bool {
    let mask = if cidr.prefix == 0 {
        0
    } else {
        u32::MAX << (32 - u32::from(cidr.prefix))
    };
    (u32::from(ip) & mask) == u32::from(cidr.network)
}

fn sort_result(result: &mut CheckResult) {
    result.hard_errors.sort();
    result.drift.sort();
    result.ipset_deltas.sort_by(|a, b| {
        (&a.set, &a.entry, a.add, &a.comment).cmp(&(&b.set, &b.entry, b.add, &b.comment))
    });
    result.peer_deltas.sort_by(|a, b| {
        (&a.user, &a.pub_key, a.action, &a.allowed_ip).cmp(&(
            &b.user,
            &b.pub_key,
            b.action,
            &b.allowed_ip,
        ))
    });
}
