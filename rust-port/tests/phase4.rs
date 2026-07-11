mod support;

use std::collections::HashMap;

use support::FakeSystem;
use wgman_rs::check::{check, compute_expected_ipsets};
use wgman_rs::model::{
    Config, ConfigSets, ResourceEntry, ResourcePort, ResourcePorts, ResourceProtocol, UserEntry,
    WGPeerDeltaAction, DB,
};

fn make_test_cfg() -> Config {
    Config {
        interface: "wg0".to_string(),
        sets: ConfigSets {
            all: "wg_allow_all".to_string(),
            ip_matrix: "wg_allow_matrix".to_string(),
            port_matrix: "wg_allow_matrix_ports".to_string(),
        },
    }
}

fn make_test_db() -> DB {
    DB {
        users: HashMap::from([
            (
                "admin".to_string(),
                UserEntry {
                    ip: "10.8.0.5".to_string(),
                    pub_key: "ADMIN_PUB=".to_string(),
                    comment: String::new(),
                    inactive: false,
                },
            ),
            (
                "alice".to_string(),
                UserEntry {
                    ip: "10.8.0.10".to_string(),
                    pub_key: "ALICE_PUB=".to_string(),
                    comment: String::new(),
                    inactive: false,
                },
            ),
            (
                "bob".to_string(),
                UserEntry {
                    ip: "10.8.0.15".to_string(),
                    pub_key: "BOB_PUB=".to_string(),
                    comment: String::new(),
                    inactive: false,
                },
            ),
        ]),
        vms: HashMap::from([
            ("sandbox".to_string(), "192.168.122.100".to_string()),
            ("mailvm".to_string(), "192.168.122.101".to_string()),
        ]),
        resources: HashMap::new(),
        access: HashMap::from([
            ("admin".to_string(), vec!["*".to_string()]),
            ("alice".to_string(), vec!["sandbox".to_string()]),
            (
                "bob".to_string(),
                vec!["sandbox".to_string(), "mailvm".to_string()],
            ),
        ]),
    }
}

fn clean_fake_system() -> FakeSystem {
    FakeSystem {
        root: true,
        subnet_result: "10.8.0.1/24".to_string(),
        wg_dump_result: concat!(
            "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
            "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
            "ALICE_PUB=\t(none)\t192.168.1.100:50001\t10.8.0.10/32\t1748000000\t102400\t204800\toff\n",
            "BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n",
        )
        .to_string(),
        ipset_results: HashMap::from([
            (
                "wg_allow_all".to_string(),
                concat!(
                    "create wg_allow_all hash:ip family inet\n",
                    "add wg_allow_all 10.8.0.5\n",
                )
                .to_string(),
            ),
            (
                "wg_allow_matrix".to_string(),
                concat!(
                    "create wg_allow_matrix hash:net,net family inet comment\n",
                    "add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n",
                    "add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n",
                    "add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n",
                )
                .to_string(),
            ),
            (
                "wg_allow_matrix_ports".to_string(),
                "create wg_allow_matrix_ports hash:ip,port,ip family inet comment\n".to_string(),
            ),
        ]),
        ..FakeSystem::default()
    }
}

fn inactive_bob_db() -> DB {
    let mut db = make_test_db();
    db.users.get_mut("bob").unwrap().inactive = true;
    db
}

fn resource_db() -> DB {
    let mut db = make_test_db();
    db.resources = HashMap::from([
        (
            "ssh@sandbox".to_string(),
            ResourceEntry {
                vm: "sandbox".to_string(),
                ports: ResourcePorts(vec![ResourcePort {
                    protocol: ResourceProtocol::Tcp,
                    port: 22,
                }]),
                comment: String::new(),
            },
        ),
        (
            "dns@mailvm".to_string(),
            ResourceEntry {
                vm: "mailvm".to_string(),
                ports: ResourcePorts(vec![ResourcePort {
                    protocol: ResourceProtocol::Udp,
                    port: 53,
                }]),
                comment: String::new(),
            },
        ),
    ]);
    db.access.insert(
        "alice".to_string(),
        vec!["ssh@sandbox".to_string(), "dns@mailvm".to_string()],
    );
    db
}

fn resource_clean_fake_system() -> FakeSystem {
    let mut system = clean_fake_system();
    system.ipset_results.insert(
        "wg_allow_matrix".to_string(),
        concat!(
            "create wg_allow_matrix hash:net,net family inet comment\n",
            "add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n",
            "add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n",
        )
        .to_string(),
    );
    system.ipset_results.insert(
        "wg_allow_matrix_ports".to_string(),
        concat!(
            "create wg_allow_matrix_ports hash:ip,port,ip family inet comment\n",
            "add wg_allow_matrix_ports 10.8.0.10,tcp:22,192.168.122.100 comment \"alice -> ssh@sandbox tcp/22\"\n",
            "add wg_allow_matrix_ports 10.8.0.10,udp:53,192.168.122.101 comment \"alice -> dns@mailvm udp/53\"\n",
        )
        .to_string(),
    );
    system
}

fn any_contains(values: &[String], needle: &str) -> bool {
    values.iter().any(|value| value.contains(needle))
}

#[test]
fn check_clean_state() {
    let result = check(&make_test_cfg(), &make_test_db(), &clean_fake_system());

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert!(result.drift.is_empty(), "{:?}", result.drift);
    assert!(result.ipset_deltas.is_empty(), "{:?}", result.ipset_deltas);
    assert!(result.peer_deltas.is_empty(), "{:?}", result.peer_deltas);
    assert!(result.wg_dump.is_some());
    assert!(result.ok());
}

#[test]
fn check_clean_state_with_resource_access() {
    let result = check(
        &make_test_cfg(),
        &resource_db(),
        &resource_clean_fake_system(),
    );

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert!(result.drift.is_empty(), "{:?}", result.drift);
    assert!(result.ipset_deltas.is_empty(), "{:?}", result.ipset_deltas);
    assert!(result.ok());
}

#[test]
fn inactive_user_absent_from_live_state_is_clean() {
    let mut system = clean_fake_system();
    system.wg_dump_result = concat!(
        "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
        "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
        "ALICE_PUB=\t(none)\t192.168.1.100:50001\t10.8.0.10/32\t1748000000\t102400\t204800\toff\n",
    )
    .to_string();
    system.ipset_results.insert(
        "wg_allow_matrix".to_string(),
        concat!(
            "create wg_allow_matrix hash:net,net family inet comment\n",
            "add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n",
        )
        .to_string(),
    );

    let result = check(&make_test_cfg(), &inactive_bob_db(), &system);

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert!(result.peer_deltas.is_empty(), "{:?}", result.peer_deltas);
    assert!(result.drift.is_empty(), "{:?}", result.drift);
    assert!(result.ok());
}

#[test]
fn inactive_user_present_in_wg_produces_remove_delta() {
    let mut system = clean_fake_system();
    system.ipset_results.insert(
        "wg_allow_matrix".to_string(),
        concat!(
            "create wg_allow_matrix hash:net,net family inet comment\n",
            "add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n",
        )
        .to_string(),
    );

    let result = check(&make_test_cfg(), &inactive_bob_db(), &system);

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert_eq!(result.peer_deltas.len(), 1);
    assert_eq!(result.peer_deltas[0].user, "bob");
    assert_eq!(result.peer_deltas[0].pub_key, "BOB_PUB=");
    assert_eq!(result.peer_deltas[0].allowed_ip, "10.8.0.15");
    assert_eq!(result.peer_deltas[0].action, WGPeerDeltaAction::Remove);
    assert!(!result.ok());
    assert!(result.clean());
}

#[test]
fn inactive_user_ipset_entries_produce_delete_deltas() {
    let mut system = clean_fake_system();
    system.wg_dump_result = concat!(
        "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
        "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
        "ALICE_PUB=\t(none)\t192.168.1.100:50001\t10.8.0.10/32\t1748000000\t102400\t204800\toff\n",
    )
    .to_string();

    let result = check(&make_test_cfg(), &inactive_bob_db(), &system);

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert!(result.peer_deltas.is_empty(), "{:?}", result.peer_deltas);
    assert!(any_contains(
        &result.drift,
        "unexpected entry 10.8.0.15,192.168.122.100"
    ));
    assert!(any_contains(
        &result.drift,
        "unexpected entry 10.8.0.15,192.168.122.101"
    ));
    assert!(result.ipset_deltas.iter().any(|delta| !delta.add
        && delta.set == "wg_allow_matrix"
        && delta.entry == "10.8.0.15,192.168.122.100"));
    assert!(result.ipset_deltas.iter().any(|delta| !delta.add
        && delta.set == "wg_allow_matrix"
        && delta.entry == "10.8.0.15,192.168.122.101"));
    assert!(result.clean());
    assert!(!result.ok());
}

#[test]
fn missing_active_wg_peer_produces_add_delta() {
    let mut system = clean_fake_system();
    system.wg_dump_result = concat!(
        "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
        "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
        "BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n",
    )
    .to_string();

    let result = check(&make_test_cfg(), &make_test_db(), &system);

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert_eq!(result.peer_deltas.len(), 1);
    assert_eq!(result.peer_deltas[0].user, "alice");
    assert_eq!(result.peer_deltas[0].action, WGPeerDeltaAction::Add);
    assert!(result.clean());
    assert!(!result.ok());
}

#[test]
fn wg_hard_errors_skip_ipset_checks() {
    let mut system = clean_fake_system();
    system
        .wg_dump_result
        .push_str("UNKNOWN_PUB=\t(none)\t(none)\t10.8.0.99/32\t0\t0\t0\toff\n");
    system.ipset_results.insert(
        "wg_allow_matrix".to_string(),
        "create wg_allow_matrix hash:net,net family inet comment\n".to_string(),
    );

    let result = check(&make_test_cfg(), &make_test_db(), &system);

    assert!(any_contains(&result.hard_errors, "not found in db users"));
    assert!(result.drift.is_empty(), "{:?}", result.drift);
    assert!(result.ipset_deltas.is_empty(), "{:?}", result.ipset_deltas);
}

#[test]
fn peer_ip_mismatch_is_hard_error() {
    let mut system = clean_fake_system();
    system.wg_dump_result = concat!(
        "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
        "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
        "ALICE_PUB=\t(none)\t(none)\t10.8.0.20/32\t0\t0\t0\toff\n",
        "BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n",
    )
    .to_string();

    let result = check(&make_test_cfg(), &make_test_db(), &system);

    assert!(any_contains(
        &result.hard_errors,
        "does not match WireGuard allowed-ip"
    ));
}

#[test]
fn user_ip_outside_interface_subnet_is_hard_error() {
    let mut db = make_test_db();
    db.users.get_mut("alice").unwrap().ip = "10.9.0.10".to_string();
    let mut system = clean_fake_system();
    system.wg_dump_result = concat!(
        "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
        "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
        "ALICE_PUB=\t(none)\t(none)\t10.9.0.10/32\t0\t0\t0\toff\n",
        "BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n",
    )
    .to_string();

    let result = check(&make_test_cfg(), &db, &system);

    assert!(any_contains(
        &result.hard_errors,
        "outside interface subnet"
    ));
}

#[test]
fn missing_and_extra_ipset_entries_produce_drift_and_deltas() {
    let mut system = clean_fake_system();
    system.ipset_results.insert(
        "wg_allow_matrix".to_string(),
        concat!(
            "create wg_allow_matrix hash:net,net family inet comment\n",
            "add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n",
            "add wg_allow_matrix 10.8.0.99,192.168.122.199 comment \"extra\"\n",
        )
        .to_string(),
    );

    let result = check(&make_test_cfg(), &make_test_db(), &system);

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert!(any_contains(
        &result.drift,
        "missing entry 10.8.0.10,192.168.122.100"
    ));
    assert!(any_contains(
        &result.drift,
        "unexpected entry 10.8.0.99,192.168.122.199"
    ));
    assert!(result.ipset_deltas.iter().any(|delta| delta.add
        && delta.entry == "10.8.0.10,192.168.122.100"
        && delta.comment == "alice -> sandbox"));
    assert!(result
        .ipset_deltas
        .iter()
        .any(|delta| !delta.add && delta.entry == "10.8.0.99,192.168.122.199"));
}

#[test]
fn resource_port_drift_uses_resource_comment() {
    let mut system = resource_clean_fake_system();
    system.ipset_results.insert(
        "wg_allow_matrix_ports".to_string(),
        concat!(
            "create wg_allow_matrix_ports hash:ip,port,ip family inet comment\n",
            "add wg_allow_matrix_ports 10.8.0.10,udp:53,192.168.122.101 comment \"alice -> dns@mailvm udp/53\"\n",
        )
        .to_string(),
    );

    let result = check(&make_test_cfg(), &resource_db(), &system);

    assert!(result.hard_errors.is_empty(), "{:?}", result.hard_errors);
    assert!(any_contains(
        &result.drift,
        "missing entry 10.8.0.10,tcp:22,192.168.122.100"
    ));
    assert!(result.ipset_deltas.iter().any(|delta| delta.add
        && delta.entry == "10.8.0.10,tcp:22,192.168.122.100"
        && delta.comment == "alice -> ssh@sandbox tcp/22"));
}

#[test]
fn missing_ipset_is_hard_error_with_init_hint() {
    let mut system = clean_fake_system();
    system.ipset_errors.insert(
        "wg_allow_all".to_string(),
        "ipset: set not found".to_string(),
    );

    let result = check(&make_test_cfg(), &make_test_db(), &system);

    assert!(any_contains(&result.hard_errors, "ipset list"));
    assert!(any_contains(&result.hard_errors, "init-ipsets"));
}

#[test]
fn ipset_type_name_and_shape_errors_are_hard_errors() {
    let cases = [
        (
            "wrong all type",
            "wg_allow_all",
            "create wg_allow_all hash:net family inet\nadd wg_allow_all 10.8.0.5\n",
            "hash:ip",
        ),
        (
            "wrong matrix name",
            "wg_allow_matrix",
            "create other_matrix hash:net,net family inet comment\nadd other_matrix 10.8.0.10,192.168.122.100\n",
            "output describes set",
        ),
        (
            "bad matrix shape",
            "wg_allow_matrix",
            "create wg_allow_matrix hash:net,net family inet comment\nadd wg_allow_matrix 10.8.0.10\n",
            "not two valid IPv4/net values",
        ),
        (
            "bad port shape",
            "wg_allow_matrix_ports",
            "create wg_allow_matrix_ports hash:ip,port,ip family inet comment\nadd wg_allow_matrix_ports 10.8.0.10,icmp:8,192.168.122.100\n",
            "invalid port",
        ),
    ];

    for (name, set_name, output, want) in cases {
        let mut system = clean_fake_system();
        system
            .ipset_results
            .insert(set_name.to_string(), output.to_string());
        let result = check(&make_test_cfg(), &make_test_db(), &system);
        assert!(
            any_contains(&result.hard_errors, want),
            "{name}: {:?}",
            result.hard_errors
        );
        assert!(result.drift.is_empty(), "{name}: {:?}", result.drift);
    }
}

#[test]
fn malformed_ipset_parse_is_hard_error() {
    let mut system = clean_fake_system();
    system.ipset_results.insert(
        "wg_allow_all".to_string(),
        "create wg_allow_all hash:ip family inet\nadd other_set 10.8.0.5\n".to_string(),
    );

    let result = check(&make_test_cfg(), &make_test_db(), &system);

    assert!(any_contains(&result.hard_errors, "parse ipset"));
}

#[test]
fn compute_expected_ipsets_covers_all_vm_resource_and_inactive_rules() {
    let expected = compute_expected_ipsets(&resource_db());

    assert_eq!(expected.all["10.8.0.5"], "");
    assert_eq!(
        expected.ip_matrix["10.8.0.15,192.168.122.100"],
        "bob -> sandbox"
    );
    assert!(!expected.ip_matrix.contains_key("10.8.0.10,192.168.122.100"));
    assert_eq!(
        expected.port_matrix["10.8.0.10,tcp:22,192.168.122.100"],
        "alice -> ssh@sandbox tcp/22"
    );
    assert_eq!(
        expected.port_matrix["10.8.0.10,udp:53,192.168.122.101"],
        "alice -> dns@mailvm udp/53"
    );

    let inactive = compute_expected_ipsets(&inactive_bob_db());
    assert!(inactive
        .ip_matrix
        .keys()
        .all(|entry| !entry.starts_with("10.8.0.15,")));
}
