use std::io::Write;

use crate::format::{
    color_green, color_grey, color_light_blue, color_red, color_yellow, TableCell,
};
use crate::model::{CheckResult, IPSetDelta, ResourcePorts, WGPeerDelta, DB};
use crate::model::{ResourceProtocol, WGPeerDeltaAction};
use crate::utils::sorted_keys;

pub fn print_check_errors<W: Write>(result: &CheckResult, writer: &mut W) {
    if !result.hard_errors.is_empty() {
        let _ = writeln!(writer, "check: hard errors:");
        for error in &result.hard_errors {
            let _ = writeln!(writer, "  - {error}");
        }
    }

    if !result.peer_deltas.is_empty() {
        let _ = writeln!(writer, "check: WireGuard drift:");
        for delta in &result.peer_deltas {
            match delta.action {
                WGPeerDeltaAction::Add => {
                    let _ = writeln!(
                        writer,
                        "  - active user {:?} peer is absent and can be added",
                        delta.user
                    );
                }
                WGPeerDeltaAction::Remove => {
                    let _ = writeln!(
                        writer,
                        "  - inactive user {:?} peer is present and can be removed",
                        delta.user
                    );
                }
            }
        }
    }

    if !result.drift.is_empty() {
        let _ = writeln!(writer, "check: ipset drift:");
        for drift in &result.drift {
            let _ = writeln!(writer, "  - {drift}");
        }
    }
}

pub fn print_ipset_deltas<W: Write>(deltas: &[IPSetDelta], writer: &mut W) {
    for delta in deltas {
        if delta.add {
            if delta.comment.is_empty() {
                let _ = writeln!(writer, "  add {} {}", delta.set, delta.entry);
            } else {
                let _ = writeln!(
                    writer,
                    "  add {} {}  # {}",
                    delta.set, delta.entry, delta.comment
                );
            }
        } else {
            let _ = writeln!(writer, "  del {} {}", delta.set, delta.entry);
        }
    }
}

pub fn print_peer_deltas<W: Write>(deltas: &[WGPeerDelta], writer: &mut W) {
    for delta in deltas {
        match delta.action {
            WGPeerDeltaAction::Add => {
                let _ = writeln!(
                    writer,
                    "  add wg peer {} {} {}",
                    delta.user, delta.pub_key, delta.allowed_ip
                );
            }
            WGPeerDeltaAction::Remove => {
                let _ = writeln!(writer, "  remove wg peer {} {}", delta.user, delta.pub_key);
            }
        }
    }
}

pub fn format_status(status: &str, color: bool) -> String {
    if !color {
        return status.to_string();
    }
    match status {
        "OK" => color_green(status),
        "FAILED" => color_red(status),
        _ => status.to_string(),
    }
}

pub fn user_name_cell(name: &str, inactive: bool, color: bool) -> TableCell {
    if !inactive {
        return TableCell::plain(name);
    }
    let plain = format!("{name}~");
    let display = if color {
        color_grey(&plain)
    } else {
        plain.clone()
    };
    TableCell::new(plain, display)
}

pub fn format_access_summary(db: &DB, targets: Option<&Vec<String>>, color: bool) -> String {
    let Some(targets) = targets else {
        return format!("({})", color_access_item(db, "none", color));
    };
    if targets.is_empty() {
        return format!("({})", color_access_item(db, "none", color));
    }

    let mut sorted = targets.clone();
    sorted.sort();
    let parts: Vec<String> = sorted
        .iter()
        .map(|target| color_access_item(db, target, color))
        .collect();
    format!("({})", parts.join(","))
}

pub fn color_access_item(db: &DB, item: &str, color: bool) -> String {
    if !color {
        return item.to_string();
    }
    match item {
        "none" => color_grey(item),
        "*" => color_red(item),
        _ if db.vms.contains_key(item) => color_yellow(item),
        _ if db.resources.contains_key(item) => color_light_blue(item),
        _ => item.to_string(),
    }
}

pub fn format_section_header(label: &str, color: bool) -> String {
    if !color {
        return format!("{label}:");
    }
    match label {
        "VMs" => color_yellow(&format!("{label}:")),
        "Resources" => color_light_blue(&format!("{label}:")),
        _ => format!("{label}:"),
    }
}

pub fn format_resource_ports(ports: &ResourcePorts) -> String {
    if ports.0.is_empty() {
        return "(none)".to_string();
    }

    let mut by_protocol: std::collections::HashMap<String, Vec<u16>> =
        std::collections::HashMap::new();
    for port in &ports.0 {
        let protocol = match port.protocol {
            ResourceProtocol::Tcp => "tcp",
            ResourceProtocol::Udp => "udp",
        };
        by_protocol
            .entry(protocol.to_string())
            .or_default()
            .push(port.port);
    }

    let mut groups = Vec::new();
    for protocol in sorted_keys(&by_protocol) {
        let mut values = by_protocol[&protocol].clone();
        values.sort_unstable();
        let ports = values
            .iter()
            .map(u16::to_string)
            .collect::<Vec<_>>()
            .join(",");
        groups.push(format!("{protocol}:{ports}"));
    }
    groups.join(" ")
}
