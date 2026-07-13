use std::collections::HashMap;

use crate::model::{Config, ExpectedIPSets, IPSetDelta, WGPeerDelta, WGPeerDeltaAction};
use crate::system::SystemAdapter;

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct AppliedStateDeltas {
    pub peer_adds: Vec<WGPeerDelta>,
    pub ipset_deltas: Vec<IPSetDelta>,
    pub peer_removes: Vec<WGPeerDelta>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ApplyError<T> {
    pub applied: T,
    pub error: String,
}

pub fn apply_ipset_deltas<S: SystemAdapter>(
    deltas: &[IPSetDelta],
    system: &S,
) -> Result<(), String> {
    apply_ipset_deltas_tracked(deltas, system)
        .map(|_| ())
        .map_err(|err| err.error)
}

pub fn apply_ipset_deltas_tracked<S: SystemAdapter>(
    deltas: &[IPSetDelta],
    system: &S,
) -> Result<Vec<IPSetDelta>, ApplyError<Vec<IPSetDelta>>> {
    let mut applied = Vec::with_capacity(deltas.len());
    for delta in deltas {
        let result = if delta.add {
            system.ipset_add(&delta.set, &delta.entry, &delta.comment)
        } else {
            system.ipset_del(&delta.set, &delta.entry)
        };
        if let Err(error) = result {
            return Err(ApplyError { applied, error });
        }
        applied.push(delta.clone());
    }
    Ok(applied)
}

pub fn invert_ipset_deltas(deltas: &[IPSetDelta]) -> Vec<IPSetDelta> {
    deltas
        .iter()
        .rev()
        .map(|delta| {
            let mut inverted = delta.clone();
            inverted.add = !inverted.add;
            inverted
        })
        .collect()
}

pub fn diff_expected_ipsets(
    cfg: &Config,
    old_expected: &ExpectedIPSets,
    new_expected: &ExpectedIPSets,
) -> Vec<IPSetDelta> {
    let mut deltas = Vec::new();
    deltas.extend(diff_expected_ipset(
        &cfg.sets.all,
        &old_expected.all,
        &new_expected.all,
    ));
    deltas.extend(diff_expected_ipset(
        &cfg.sets.ip_matrix,
        &old_expected.ip_matrix,
        &new_expected.ip_matrix,
    ));
    deltas.extend(diff_expected_ipset(
        &cfg.sets.port_matrix,
        &old_expected.port_matrix,
        &new_expected.port_matrix,
    ));
    deltas.sort_by(|a, b| (&a.set, &a.entry, a.add).cmp(&(&b.set, &b.entry, b.add)));
    deltas
}

pub fn diff_expected_ipset(
    set_name: &str,
    old_expected: &HashMap<String, String>,
    new_expected: &HashMap<String, String>,
) -> Vec<IPSetDelta> {
    let mut deltas = Vec::new();
    for (entry, comment) in new_expected {
        if !old_expected.contains_key(entry) {
            deltas.push(IPSetDelta {
                set: set_name.to_string(),
                entry: entry.clone(),
                comment: comment.clone(),
                add: true,
            });
        }
    }
    for (entry, comment) in old_expected {
        if !new_expected.contains_key(entry) {
            deltas.push(IPSetDelta {
                set: set_name.to_string(),
                entry: entry.clone(),
                comment: comment.clone(),
                add: false,
            });
        }
    }
    deltas
}

pub fn apply_peer_deltas<S: SystemAdapter>(
    iface: &str,
    deltas: &[WGPeerDelta],
    system: &S,
) -> Result<(), String> {
    apply_peer_deltas_tracked(iface, deltas, system)
        .map(|_| ())
        .map_err(|err| err.error)
}

pub fn apply_peer_deltas_tracked<S: SystemAdapter>(
    iface: &str,
    deltas: &[WGPeerDelta],
    system: &S,
) -> Result<Vec<WGPeerDelta>, ApplyError<Vec<WGPeerDelta>>> {
    let mut applied = Vec::with_capacity(deltas.len());
    for delta in deltas {
        let result = match delta.action {
            WGPeerDeltaAction::Add => system
                .wg_set_peer(iface, &delta.pub_key, &delta.allowed_ip)
                .map_err(|err| format!("add WireGuard peer for {:?}: {err}", delta.user)),
            WGPeerDeltaAction::Remove => system
                .wg_del_peer(iface, &delta.pub_key)
                .map_err(|err| format!("remove WireGuard peer for {:?}: {err}", delta.user)),
        };
        if let Err(error) = result {
            return Err(ApplyError { applied, error });
        }
        applied.push(delta.clone());
    }
    Ok(applied)
}

pub fn invert_peer_deltas(deltas: &[WGPeerDelta]) -> Vec<WGPeerDelta> {
    deltas
        .iter()
        .rev()
        .map(|delta| {
            let mut inverted = delta.clone();
            inverted.action = match inverted.action {
                WGPeerDeltaAction::Add => WGPeerDeltaAction::Remove,
                WGPeerDeltaAction::Remove => WGPeerDeltaAction::Add,
            };
            inverted
        })
        .collect()
}

pub fn apply_state_deltas<S: SystemAdapter>(
    iface: &str,
    ipset_deltas: &[IPSetDelta],
    peer_deltas: &[WGPeerDelta],
    system: &S,
) -> Result<(), String> {
    apply_state_deltas_tracked(iface, ipset_deltas, peer_deltas, system)
        .map(|_| ())
        .map_err(|err| err.error)
}

pub fn apply_state_deltas_tracked<S: SystemAdapter>(
    iface: &str,
    ipset_deltas: &[IPSetDelta],
    peer_deltas: &[WGPeerDelta],
    system: &S,
) -> Result<AppliedStateDeltas, ApplyError<AppliedStateDeltas>> {
    let mut applied = AppliedStateDeltas::default();

    let peer_adds = filter_peer_deltas(peer_deltas, WGPeerDeltaAction::Add);
    applied.peer_adds =
        apply_peer_deltas_tracked(iface, &peer_adds, system).map_err(|err| ApplyError {
            applied: AppliedStateDeltas {
                peer_adds: err.applied,
                ..AppliedStateDeltas::default()
            },
            error: err.error,
        })?;

    applied.ipset_deltas =
        apply_ipset_deltas_tracked(ipset_deltas, system).map_err(|err| ApplyError {
            applied: AppliedStateDeltas {
                peer_adds: applied.peer_adds.clone(),
                ipset_deltas: err.applied,
                peer_removes: Vec::new(),
            },
            error: err.error,
        })?;

    let peer_removes = filter_peer_deltas(peer_deltas, WGPeerDeltaAction::Remove);
    applied.peer_removes =
        apply_peer_deltas_tracked(iface, &peer_removes, system).map_err(|err| ApplyError {
            applied: AppliedStateDeltas {
                peer_adds: applied.peer_adds.clone(),
                ipset_deltas: applied.ipset_deltas.clone(),
                peer_removes: err.applied,
            },
            error: err.error,
        })?;

    Ok(applied)
}

pub fn rollback_state_deltas<S: SystemAdapter>(
    iface: &str,
    applied: &AppliedStateDeltas,
    system: &S,
) -> Result<(), String> {
    let mut errors = Vec::new();

    if let Err(err) = apply_peer_deltas(iface, &invert_peer_deltas(&applied.peer_removes), system) {
        errors.push(err);
    }
    if let Err(err) = apply_ipset_deltas(&invert_ipset_deltas(&applied.ipset_deltas), system) {
        errors.push(err);
    }
    if let Err(err) = apply_peer_deltas(iface, &invert_peer_deltas(&applied.peer_adds), system) {
        errors.push(err);
    }

    if errors.is_empty() {
        Ok(())
    } else {
        Err(errors.join("; "))
    }
}

fn filter_peer_deltas(deltas: &[WGPeerDelta], action: WGPeerDeltaAction) -> Vec<WGPeerDelta> {
    deltas
        .iter()
        .filter(|delta| delta.action == action)
        .cloned()
        .collect()
}
