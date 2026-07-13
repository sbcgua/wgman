mod support;

use std::collections::HashMap;
use std::ffi::OsString;
use std::fs;
use std::io::Cursor;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use support::FakeSystem;
use tempfile::TempDir;
use wgman_rs::cli::App;

fn fixed_now() -> SystemTime {
    UNIX_EPOCH + Duration::from_secs(1_748_001_000)
}

fn run(args: &[&str], system: FakeSystem, input: &str) -> (i32, String, String, FakeSystem) {
    let app = App::new_with_now(system, fixed_now);
    let mut stdin = Cursor::new(input.as_bytes());
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();
    let code = app.run_with_input(
        args.iter().map(OsString::from),
        &mut stdin,
        &mut stdout,
        &mut stderr,
    );
    let system = app.into_system();
    (
        code,
        String::from_utf8(stdout).expect("stdout is utf8"),
        String::from_utf8(stderr).expect("stderr is utf8"),
        system,
    )
}

fn write_config_only() -> TempDir {
    let dir = tempfile::tempdir().expect("tempdir");
    fs::write(
        dir.path().join("config.yaml"),
        concat!(
            "interface: wg0\n",
            "sets:\n",
            "  all: wg_allow_all\n",
            "  ip_matrix: wg_allow_matrix\n",
            "  port_matrix: wg_allow_matrix_ports\n",
        ),
    )
    .expect("write config");
    dir
}

fn write_config_dir() -> TempDir {
    let dir = write_config_only();
    fs::write(
        dir.path().join("db.yaml"),
        concat!(
            "users:\n",
            "  admin:\n",
            "    ip: 10.8.0.5\n",
            "    pub: ADMIN_PUB=\n",
            "  alice:\n",
            "    ip: 10.8.0.10\n",
            "    pub: ALICE_PUB=\n",
            "  bob:\n",
            "    ip: 10.8.0.15\n",
            "    pub: BOB_PUB=\n",
            "vms:\n",
            "  sandbox: 192.168.122.100\n",
            "  mailvm: 192.168.122.101\n",
            "resources: {}\n",
            "access:\n",
            "  admin:\n",
            "    - \"*\"\n",
            "  alice:\n",
            "    - sandbox\n",
            "  bob:\n",
            "    - sandbox\n",
            "    - mailvm\n",
        ),
    )
    .expect("write db");
    dir
}

fn write_inactive_bob_config_dir() -> TempDir {
    let dir = write_config_only();
    fs::write(
        dir.path().join("db.yaml"),
        concat!(
            "users:\n",
            "  admin:\n",
            "    ip: 10.8.0.5\n",
            "    pub: ADMIN_PUB=\n",
            "  alice:\n",
            "    ip: 10.8.0.10\n",
            "    pub: ALICE_PUB=\n",
            "  bob:\n",
            "    ip: 10.8.0.15\n",
            "    pub: BOB_PUB=\n",
            "    comment: on leave\n",
            "    inactive: true\n",
            "vms:\n",
            "  sandbox: 192.168.122.100\n",
            "  mailvm: 192.168.122.101\n",
            "resources: {}\n",
            "access:\n",
            "  admin:\n",
            "    - \"*\"\n",
            "  alice:\n",
            "    - sandbox\n",
            "  bob:\n",
            "    - sandbox\n",
            "    - mailvm\n",
        ),
    )
    .expect("write db");
    dir
}

fn clean_system() -> FakeSystem {
    FakeSystem {
        root: true,
        subnet_result: "10.8.0.1/24".to_string(),
        wg_dump_result: concat!(
            "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
            "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
            "ALICE_PUB=\t(none)\t(none)\t10.8.0.10/32\t0\t0\t0\toff\n",
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

fn missing_bob_peer_system() -> FakeSystem {
    let mut system = clean_system();
    system.wg_dump_result = concat!(
        "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
        "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
        "ALICE_PUB=\t(none)\t(none)\t10.8.0.10/32\t0\t0\t0\toff\n",
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
    system
}

fn inactive_bob_drift_system() -> FakeSystem {
    clean_system()
}

#[test]
fn init_ipsets_creates_configured_sets_with_types_and_comments() {
    let dir = write_config_only();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr, system) = run(
        &["wgman-rs", "init-ipsets", "--config-dir", &dir_string],
        FakeSystem {
            root: true,
            ..FakeSystem::default()
        },
        "",
    );

    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("wg_allow_all"));
    assert!(stdout.contains("init-ipsets: OK"));
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "create:wg_allow_all:hash:ip:nocomment",
            "create:wg_allow_matrix:hash:net,net:comment",
            "create:wg_allow_matrix_ports:hash:ip,port,ip:comment",
        ]
    );
}

#[test]
fn init_ipsets_rejects_args_root_dry_run_and_propagates_create_error() {
    let dir = write_config_only();
    let dir_string = dir.path().display().to_string();

    let (code, _, stderr, _) = run(
        &[
            "wgman-rs",
            "init-ipsets",
            "--config-dir",
            &dir_string,
            "extra",
        ],
        FakeSystem {
            root: true,
            ..FakeSystem::default()
        },
        "",
    );
    assert_eq!(code, 2);
    assert!(stderr.contains("no positional"));

    let (code, _, stderr, _) = run(
        &["wgman-rs", "init-ipsets", "--config-dir", &dir_string],
        FakeSystem::default(),
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("root"));

    let (code, _, stderr, _) = run(
        &["wgman-rs", "init-ipsets", "--dry-run"],
        FakeSystem::default(),
        "",
    );
    assert_eq!(code, 2);
    assert!(stderr.contains("does not support --dry-run"));

    let (code, _, stderr, _) = run(
        &["wgman-rs", "init-ipsets", "--config-dir", &dir_string],
        FakeSystem {
            root: true,
            ipset_create_error: Some("ipset: kernel error".to_string()),
            ..FakeSystem::default()
        },
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("kernel error"));
}

#[test]
fn deploy_no_changes_and_hard_errors() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr, system) = run(
        &["wgman-rs", "deploy", "--config-dir", &dir_string],
        clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert_eq!(stdout, "deploy: no changes needed\n");
    assert!(system.applied_ops.borrow().is_empty());

    let mut hard = clean_system();
    hard.wg_dump_error = Some("connection refused".to_string());
    let (code, _, stderr, _) = run(
        &["wgman-rs", "deploy", "--config-dir", &dir_string],
        hard,
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("deploy: hard errors:"));
    assert!(stderr.contains("FAILED"));
}

#[test]
fn deploy_dry_run_reports_peer_and_ipset_plans_without_mutating() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "deploy",
            "--dry-run",
            "--config-dir",
            &dir_string,
        ],
        missing_bob_peer_system(),
        "",
    );

    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("deploy: planned changes"));
    assert!(stdout.contains("add wg peer bob BOB_PUB= 10.8.0.15"));
    assert!(stdout.contains("add wg_allow_matrix 10.8.0.15,192.168.122.100"));
    assert!(stdout.contains("dry-run"));
    assert!(system.applied_ops.borrow().is_empty());
}

#[test]
fn deploy_prompt_abort_and_yes_apply_expected_order() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, _, system) = run(
        &["wgman-rs", "deploy", "--config-dir", &dir_string],
        missing_bob_peer_system(),
        "n\n",
    );
    assert_eq!(code, 0);
    assert!(stdout.contains("Apply these changes? [y/N] deploy: aborted"));
    assert!(system.applied_ops.borrow().is_empty());

    let (code, stdout, stderr, system) = run(
        &["wgman-rs", "deploy", "--yes", "--config-dir", &dir_string],
        missing_bob_peer_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("deploy: applied"));
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "wgset:wg0:BOB_PUB=:10.8.0.15",
            "add:wg_allow_matrix:10.8.0.15,192.168.122.100:bob -> sandbox",
            "add:wg_allow_matrix:10.8.0.15,192.168.122.101:bob -> mailvm",
        ]
    );
}

#[test]
fn deploy_prompt_yes_applies_changes() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr, system) = run(
        &["wgman-rs", "deploy", "--config-dir", &dir_string],
        missing_bob_peer_system(),
        "y\n",
    );

    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("Apply these changes? [y/N] deploy: applied"));
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "wgset:wg0:BOB_PUB=:10.8.0.15",
            "add:wg_allow_matrix:10.8.0.15,192.168.122.100:bob -> sandbox",
            "add:wg_allow_matrix:10.8.0.15,192.168.122.101:bob -> mailvm",
        ]
    );
}

#[test]
fn deploy_removes_inactive_ipsets_before_peer_and_reports_failures() {
    let dir = write_inactive_bob_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr, system) = run(
        &["wgman-rs", "deploy", "--yes", "--config-dir", &dir_string],
        inactive_bob_drift_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("remove wg peer bob BOB_PUB="));
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "del:wg_allow_matrix:10.8.0.15,192.168.122.100",
            "del:wg_allow_matrix:10.8.0.15,192.168.122.101",
            "wgdel:wg0:BOB_PUB=",
        ]
    );

    let (code, _, stderr, _) = run(
        &["wgman-rs", "deploy", "--yes", "--config-dir", &dir_string],
        FakeSystem {
            wg_del_error: Some("wg: operation failed".to_string()),
            ..inactive_bob_drift_system()
        },
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("operation failed"));
}

#[test]
fn deploy_deletes_unexpected_ipset_entries() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();
    let mut system = clean_system();
    system.ipset_results.insert(
        "wg_allow_matrix".to_string(),
        concat!(
            "create wg_allow_matrix hash:net,net family inet comment\n",
            "add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n",
            "add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n",
            "add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n",
            "add wg_allow_matrix 10.8.0.99,192.168.122.199 comment \"extra\"\n",
        )
        .to_string(),
    );

    let (code, stdout, stderr, system) = run(
        &["wgman-rs", "deploy", "--yes", "--config-dir", &dir_string],
        system,
        "",
    );

    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("del wg_allow_matrix 10.8.0.99,192.168.122.199"));
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        ["del:wg_allow_matrix:10.8.0.99,192.168.122.199"]
    );
}
