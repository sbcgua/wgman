mod support;

use std::collections::HashMap;

use support::FakeSystem;
use wgman_rs::deploy::{
    apply_ipset_deltas_tracked, apply_peer_deltas_tracked, apply_state_deltas,
    apply_state_deltas_tracked, diff_expected_ipset, diff_expected_ipsets, invert_ipset_deltas,
    invert_peer_deltas, rollback_state_deltas, AppliedStateDeltas,
};
use wgman_rs::model::{
    Config, ConfigSets, ExpectedIPSets, IPSetDelta, WGPeerDelta, WGPeerDeltaAction,
};

fn cfg() -> Config {
    Config {
        interface: "wg0".to_string(),
        sets: ConfigSets {
            all: "wg_allow_all".to_string(),
            ip_matrix: "wg_allow_matrix".to_string(),
            port_matrix: "wg_allow_matrix_ports".to_string(),
        },
    }
}

fn ipset_delta(set: &str, entry: &str, comment: &str, add: bool) -> IPSetDelta {
    IPSetDelta {
        set: set.to_string(),
        entry: entry.to_string(),
        comment: comment.to_string(),
        add,
    }
}

fn peer_delta(
    user: &str,
    pub_key: &str,
    allowed_ip: &str,
    action: WGPeerDeltaAction,
) -> WGPeerDelta {
    WGPeerDelta {
        user: user.to_string(),
        pub_key: pub_key.to_string(),
        allowed_ip: allowed_ip.to_string(),
        action,
    }
}

#[test]
fn apply_ipset_deltas_tracked_stops_at_first_error() {
    let system = FakeSystem {
        ipset_del_error: Some("delete failed".to_string()),
        ..FakeSystem::default()
    };
    let deltas = vec![
        ipset_delta("allow", "10.8.0.10", "alice", true),
        ipset_delta("allow", "10.8.0.20", "", false),
        ipset_delta("allow", "10.8.0.30", "", true),
    ];

    let err = apply_ipset_deltas_tracked(&deltas, &system).unwrap_err();

    assert!(err.error.contains("delete failed"), "{err:?}");
    assert_eq!(err.applied, deltas[..1]);
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        ["add:allow:10.8.0.10:alice"]
    );
}

#[test]
fn invert_ipset_deltas_reverses_order_and_operation() {
    let deltas = vec![
        ipset_delta("allow", "10.8.0.10", "alice", true),
        ipset_delta("allow", "10.8.0.20", "", false),
    ];

    let got = invert_ipset_deltas(&deltas);
    let want = vec![
        ipset_delta("allow", "10.8.0.20", "", true),
        ipset_delta("allow", "10.8.0.10", "alice", false),
    ];

    assert_eq!(got, want);
    assert!(deltas[0].add);
    assert!(!deltas[1].add);
}

#[test]
fn invert_peer_deltas_reverses_order_and_operation() {
    let deltas = vec![
        peer_delta("alice", "ALICE", "10.8.0.10", WGPeerDeltaAction::Add),
        peer_delta("bob", "BOB", "10.8.0.15", WGPeerDeltaAction::Remove),
    ];

    let got = invert_peer_deltas(&deltas);
    let want = vec![
        peer_delta("bob", "BOB", "10.8.0.15", WGPeerDeltaAction::Add),
        peer_delta("alice", "ALICE", "10.8.0.10", WGPeerDeltaAction::Remove),
    ];

    assert_eq!(got, want);
    assert_eq!(deltas[0].action, WGPeerDeltaAction::Add);
    assert_eq!(deltas[1].action, WGPeerDeltaAction::Remove);
}

#[test]
fn apply_peer_deltas_tracked_stops_at_first_error() {
    let system = FakeSystem {
        wg_del_error: Some("delete failed".to_string()),
        ..FakeSystem::default()
    };
    let deltas = vec![
        peer_delta("alice", "ALICE", "10.8.0.10", WGPeerDeltaAction::Add),
        peer_delta("bob", "BOB", "10.8.0.15", WGPeerDeltaAction::Remove),
    ];

    let err = apply_peer_deltas_tracked("wg0", &deltas, &system).unwrap_err();

    assert!(err.error.contains("delete failed"), "{err:?}");
    assert_eq!(err.applied, deltas[..1]);
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        ["wgset:wg0:ALICE:10.8.0.10"]
    );
}

#[test]
fn diff_expected_ipsets_builds_stable_deltas() {
    let old_expected = ExpectedIPSets {
        all: HashMap::from([("10.8.0.5".to_string(), String::new())]),
        ip_matrix: HashMap::from([(
            "10.8.0.10,192.168.122.100".to_string(),
            "alice -> sandbox".to_string(),
        )]),
        port_matrix: HashMap::from([(
            "10.8.0.10,tcp:22,192.168.122.100".to_string(),
            "alice -> ssh@sandbox tcp/22".to_string(),
        )]),
    };
    let new_expected = ExpectedIPSets {
        all: HashMap::from([("10.8.0.6".to_string(), String::new())]),
        ip_matrix: HashMap::from([(
            "10.8.0.10,192.168.122.101".to_string(),
            "alice -> mailvm".to_string(),
        )]),
        port_matrix: HashMap::from([(
            "10.8.0.10,udp:53,192.168.122.101".to_string(),
            "alice -> dns@mailvm udp/53".to_string(),
        )]),
    };

    let got = diff_expected_ipsets(&cfg(), &old_expected, &new_expected);
    let want = vec![
        ipset_delta("wg_allow_all", "10.8.0.5", "", false),
        ipset_delta("wg_allow_all", "10.8.0.6", "", true),
        ipset_delta(
            "wg_allow_matrix",
            "10.8.0.10,192.168.122.100",
            "alice -> sandbox",
            false,
        ),
        ipset_delta(
            "wg_allow_matrix",
            "10.8.0.10,192.168.122.101",
            "alice -> mailvm",
            true,
        ),
        ipset_delta(
            "wg_allow_matrix_ports",
            "10.8.0.10,tcp:22,192.168.122.100",
            "alice -> ssh@sandbox tcp/22",
            false,
        ),
        ipset_delta(
            "wg_allow_matrix_ports",
            "10.8.0.10,udp:53,192.168.122.101",
            "alice -> dns@mailvm udp/53",
            true,
        ),
    ];

    assert_eq!(got, want);
}

#[test]
fn diff_expected_ipsets_orders_deletes_before_adds_for_matching_keys() {
    let shared_entry = "10.8.0.10,192.168.122.100";
    let old_expected = ExpectedIPSets {
        all: HashMap::from([(shared_entry.to_string(), "old comment".to_string())]),
        ..ExpectedIPSets::default()
    };
    let new_expected = ExpectedIPSets {
        ip_matrix: HashMap::from([(shared_entry.to_string(), "alice -> sandbox".to_string())]),
        ..ExpectedIPSets::default()
    };
    let config = Config {
        interface: "wg0".to_string(),
        sets: ConfigSets {
            all: "same_set".to_string(),
            ip_matrix: "same_set".to_string(),
            port_matrix: "port_set".to_string(),
        },
    };

    let got = diff_expected_ipsets(&config, &old_expected, &new_expected);

    assert_eq!(
        got,
        [
            ipset_delta("same_set", shared_entry, "old comment", false),
            ipset_delta("same_set", shared_entry, "alice -> sandbox", true),
        ]
    );
}

#[test]
fn diff_expected_ipset_no_changes() {
    let expected = HashMap::from([(
        "10.8.0.10,192.168.122.100".to_string(),
        "alice -> sandbox".to_string(),
    )]);

    let got = diff_expected_ipset("wg_allow_matrix", &expected, &expected);

    assert!(got.is_empty(), "{got:?}");
}

#[test]
fn apply_state_deltas_uses_dependency_order() {
    let system = FakeSystem::default();
    let ipset_deltas = vec![
        ipset_delta("allow", "10.8.0.10", "alice", true),
        ipset_delta("allow", "10.8.0.20", "", false),
    ];
    let peer_deltas = vec![
        peer_delta("bob", "BOB", "10.8.0.20", WGPeerDeltaAction::Remove),
        peer_delta("alice", "ALICE", "10.8.0.10", WGPeerDeltaAction::Add),
    ];

    apply_state_deltas("wg0", &ipset_deltas, &peer_deltas, &system).unwrap();

    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "wgset:wg0:ALICE:10.8.0.10",
            "add:allow:10.8.0.10:alice",
            "del:allow:10.8.0.20",
            "wgdel:wg0:BOB",
        ]
    );
}

#[test]
fn apply_state_deltas_stops_before_peer_removal_on_ipset_error() {
    let system = FakeSystem {
        ipset_add_error: Some("add failed".to_string()),
        ..FakeSystem::default()
    };

    let err = apply_state_deltas(
        "wg0",
        &[ipset_delta("allow", "10.8.0.10", "", true)],
        &[
            peer_delta("alice", "ALICE", "10.8.0.10", WGPeerDeltaAction::Add),
            peer_delta("bob", "BOB", "10.8.0.20", WGPeerDeltaAction::Remove),
        ],
        &system,
    )
    .unwrap_err();

    assert!(err.contains("add failed"), "{err}");
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        ["wgset:wg0:ALICE:10.8.0.10"]
    );
}

#[test]
fn apply_state_deltas_tracked_returns_completed_operations() {
    let system = FakeSystem {
        wg_del_error: Some("delete failed".to_string()),
        ..FakeSystem::default()
    };
    let ipset_deltas = vec![ipset_delta("allow", "10.8.0.10", "alice", true)];
    let peer_deltas = vec![
        peer_delta("alice", "ALICE", "10.8.0.10", WGPeerDeltaAction::Add),
        peer_delta("bob", "BOB", "10.8.0.20", WGPeerDeltaAction::Remove),
    ];

    let err = apply_state_deltas_tracked("wg0", &ipset_deltas, &peer_deltas, &system).unwrap_err();

    assert!(err.error.contains("delete failed"), "{err:?}");
    assert_eq!(err.applied.peer_adds, peer_deltas[..1]);
    assert_eq!(err.applied.ipset_deltas, ipset_deltas);
    assert!(err.applied.peer_removes.is_empty());
}

#[test]
fn apply_state_deltas_tracked_returns_completed_struct() {
    let system = FakeSystem::default();
    let ipset_deltas = vec![ipset_delta("allow", "10.8.0.10", "alice", true)];
    let peer_deltas = vec![
        peer_delta("alice", "ALICE", "10.8.0.10", WGPeerDeltaAction::Add),
        peer_delta("bob", "BOB", "10.8.0.20", WGPeerDeltaAction::Remove),
    ];

    let applied = apply_state_deltas_tracked("wg0", &ipset_deltas, &peer_deltas, &system).unwrap();

    assert_eq!(applied.peer_adds, peer_deltas[..1]);
    assert_eq!(applied.ipset_deltas, ipset_deltas);
    assert_eq!(applied.peer_removes, peer_deltas[1..]);
}

#[test]
fn rollback_state_deltas_uses_reverse_dependency_order() {
    let system = FakeSystem::default();
    let applied = AppliedStateDeltas {
        peer_adds: vec![peer_delta(
            "alice",
            "ALICE",
            "10.8.0.10",
            WGPeerDeltaAction::Add,
        )],
        ipset_deltas: vec![ipset_delta("allow", "10.8.0.10", "alice", true)],
        peer_removes: vec![peer_delta(
            "bob",
            "BOB",
            "10.8.0.20",
            WGPeerDeltaAction::Remove,
        )],
    };

    rollback_state_deltas("wg0", &applied, &system).unwrap();

    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "wgset:wg0:BOB:10.8.0.20",
            "del:allow:10.8.0.10",
            "wgdel:wg0:ALICE",
        ]
    );
}
