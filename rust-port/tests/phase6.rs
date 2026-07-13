mod support;

use std::collections::HashMap;
use std::ffi::OsString;
use std::fs;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use support::FakeSystem;
use tempfile::TempDir;
use wgman_rs::cli::{App, HELP_TEXT};
use wgman_rs::commands::list::run_list_with_color;
use wgman_rs::commands::show::run_show_with_color;
use wgman_rs::format::{ANSI_DIM_CYAN, ANSI_GREY, ANSI_LIGHT_BLUE, ANSI_RED, ANSI_RESET};
use wgman_rs::model::{
    CheckResult, ResourceEntry, ResourcePort, ResourcePorts, ResourceProtocol, UserEntry, DB,
};
use wgman_rs::parse_wg::{WGDumpResult, WGPeer};

fn fixed_now() -> SystemTime {
    UNIX_EPOCH + Duration::from_secs(1_748_001_000)
}

fn run(args: &[&str], system: FakeSystem) -> (i32, String, String) {
    let app = App::new_with_now(system, fixed_now);
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();
    let code = app.run(args.iter().map(OsString::from), &mut stdout, &mut stderr);
    (
        code,
        String::from_utf8(stdout).expect("stdout is utf8"),
        String::from_utf8(stderr).expect("stderr is utf8"),
    )
}

fn write_config_dir() -> TempDir {
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
            "resources:\n",
            "  ssh@sandbox:\n",
            "    vm: sandbox\n",
            "    ports: 22\n",
            "  dns@mailvm:\n",
            "    vm: mailvm\n",
            "    ports:\n",
            "      - udp:53\n",
            "      - tcp:53\n",
            "access:\n",
            "  admin:\n",
            "    - \"*\"\n",
            "  alice:\n",
            "    - ssh@sandbox\n",
            "    - dns@mailvm\n",
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
                    "add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n",
                    "add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n",
                )
                .to_string(),
            ),
            (
                "wg_allow_matrix_ports".to_string(),
                concat!(
                    "create wg_allow_matrix_ports hash:ip,port,ip family inet comment\n",
                    "add wg_allow_matrix_ports 10.8.0.10,tcp:53,192.168.122.101 comment \"alice -> dns@mailvm tcp/53\"\n",
                    "add wg_allow_matrix_ports 10.8.0.10,udp:53,192.168.122.101 comment \"alice -> dns@mailvm udp/53\"\n",
                    "add wg_allow_matrix_ports 10.8.0.10,tcp:22,192.168.122.100 comment \"alice -> ssh@sandbox tcp/22\"\n",
                )
                .to_string(),
            ),
        ]),
        ..FakeSystem::default()
    }
}

fn make_db() -> DB {
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
        resources: HashMap::from([
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
                    ports: ResourcePorts(vec![
                        ResourcePort {
                            protocol: ResourceProtocol::Udp,
                            port: 53,
                        },
                        ResourcePort {
                            protocol: ResourceProtocol::Tcp,
                            port: 53,
                        },
                    ]),
                    comment: String::new(),
                },
            ),
        ]),
        access: HashMap::from([
            ("admin".to_string(), vec!["*".to_string()]),
            (
                "alice".to_string(),
                vec!["ssh@sandbox".to_string(), "dns@mailvm".to_string()],
            ),
            (
                "bob".to_string(),
                vec!["sandbox".to_string(), "mailvm".to_string()],
            ),
        ]),
    }
}

fn clean_result() -> CheckResult {
    CheckResult {
        wg_dump: Some(WGDumpResult {
            server_pub_key: "SERVER_PUB=".to_string(),
            peers: vec![
                WGPeer {
                    public_key: "ADMIN_PUB=".to_string(),
                    preshared_key: "(none)".to_string(),
                    endpoint: "(none)".to_string(),
                    allowed_ip: "10.8.0.5".to_string(),
                    latest_handshake: 0,
                    rx_bytes: 0,
                    tx_bytes: 0,
                    keepalive: "off".to_string(),
                },
                WGPeer {
                    public_key: "ALICE_PUB=".to_string(),
                    preshared_key: "(none)".to_string(),
                    endpoint: "192.168.1.100:50001".to_string(),
                    allowed_ip: "10.8.0.10".to_string(),
                    latest_handshake: 1_748_000_000,
                    rx_bytes: 102400,
                    tx_bytes: 204800,
                    keepalive: "off".to_string(),
                },
            ],
        }),
        ..CheckResult::default()
    }
}

#[test]
fn help_unknown_and_argument_errors_have_go_exit_shape() {
    let (code, stdout, stderr) = run(&["wgman-rs", "show", "--help"], FakeSystem::default());
    assert_eq!(code, 0);
    assert_eq!(stdout, HELP_TEXT);
    assert_eq!(stderr, "");

    let (code, stdout, stderr) = run(&["wgman-rs", "bogus"], FakeSystem::default());
    assert_eq!(code, 2);
    assert_eq!(stdout, "");
    assert!(stderr.contains("unknown command"));

    let (code, _, stderr) = run(&["wgman-rs", "list", "alice", "bob"], FakeSystem::default());
    assert_eq!(code, 2);
    assert!(stderr.contains("at most one positional"));
}

#[test]
fn global_flags_and_comment_shape_are_parsed_before_and_after_command() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr) = run(
        &["wgman-rs", "--config-dir", &dir_string, "list", "alice"],
        clean_system(),
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("alice:"));
    assert!(stdout.contains("dns@mailvm"));

    let (code, stdout, stderr) = run(
        &["wgman-rs", "list", "--config-dir", &dir_string, "alice"],
        clean_system(),
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("ssh@sandbox"));

    let (code, _, stderr) = run(&["wgman-rs", "-c", "note", "check"], FakeSystem::default());
    assert_eq!(code, 2);
    assert!(stderr.contains("only supported by create/add"));

    let (code, _, stderr) = run(
        &["wgman-rs", "create", "-c", "  ", "alice"],
        FakeSystem::default(),
    );
    assert_eq!(code, 2);
    assert!(stderr.contains("comment must not be empty"));
}

#[test]
fn bool_flags_accept_go_assignment_forms() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr) = run(
        &[
            "wgman-rs",
            "-config-dir",
            &dir_string,
            "-no-color=true",
            "check",
        ],
        clean_system(),
    );
    assert_eq!(code, 0, "{stderr}");
    assert_eq!(stdout, "check: OK\n");

    let (code, stdout, stderr) = run(
        &[
            "wgman-rs",
            "--config-dir",
            &dir_string,
            "--dry-run=false",
            "check",
        ],
        clean_system(),
    );
    assert_eq!(code, 0, "{stderr}");
    assert_eq!(stdout, "check: OK\n");

    let (code, _, stderr) = run(&["wgman-rs", "--yes=maybe", "check"], FakeSystem::default());
    assert_eq!(code, 2);
    assert!(stderr.contains("invalid boolean value"));
}

#[test]
fn read_only_commands_reject_dry_run_and_require_root() {
    for command in ["check", "list", "show"] {
        let (code, _, stderr) = run(&["wgman-rs", command, "--dry-run"], FakeSystem::default());
        assert_eq!(code, 2, "{command}");
        assert!(
            stderr.contains("does not support --dry-run"),
            "{command}: {stderr}"
        );
    }

    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();
    let (code, _, stderr) = run(
        &["wgman-rs", "check", "--config-dir", &dir_string],
        FakeSystem::default(),
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("root"));
}

#[test]
fn check_reports_ok_failed_color_and_deltas() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();
    let mut system = clean_system();
    system.stdout_tty = true;

    let (code, stdout, stderr) = run(&["wgman-rs", "check", "--config-dir", &dir_string], system);
    assert_eq!(code, 0, "{stderr}");
    assert_eq!(stdout, "check: \x1b[32mOK\x1b[0m\n");

    let mut drift = clean_system();
    drift.ipset_results.insert(
        "wg_allow_matrix".to_string(),
        "create wg_allow_matrix hash:net,net family inet comment\n".to_string(),
    );
    drift.stderr_tty = true;
    let (code, _, stderr) = run(&["wgman-rs", "check", "--config-dir", &dir_string], drift);
    assert_eq!(code, 1);
    assert!(stderr.contains("check: ipset drift:"));
    assert!(!stderr.contains("check: planned ipset deltas:"));
    assert!(!stderr.contains("add wg_allow_matrix"));
    assert!(stderr.contains("\x1b[31mFAILED\x1b[0m"));
}

#[test]
fn list_outputs_users_resources_filter_and_color() {
    let db = make_db();
    let mut stdout = Vec::new();
    let code = run_list_with_color(
        &db,
        &CheckResult::default(),
        "",
        &mut stdout,
        &mut Vec::new(),
        true,
    );
    assert_eq!(code, 0);
    let out = String::from_utf8(stdout).unwrap();
    assert!(out.contains("Users:"));
    assert!(out.contains(&format!("({ANSI_RED}*{ANSI_RESET})")));
    assert!(out.contains(&format!("{ANSI_LIGHT_BLUE}Resources:{ANSI_RESET}")));
    assert!(out.contains("tcp:53 udp:53"));
    assert!(strip_ansi(&out).contains("(dns@mailvm,ssh@sandbox)"));

    let mut filtered = Vec::new();
    let code = run_list_with_color(
        &db,
        &CheckResult::default(),
        "alice",
        &mut filtered,
        &mut Vec::new(),
        false,
    );
    assert_eq!(code, 0);
    let out = String::from_utf8(filtered).unwrap();
    assert!(out.contains("alice:"));
    assert!(out.contains("dns@mailvm"));
    assert!(!out.contains("mailvm\n  sandbox"));
}

#[test]
fn show_maps_peers_and_colorizes_values_without_layout_drift() {
    let db = make_db();
    let result = clean_result();
    let mut plain = Vec::new();
    let mut colored = Vec::new();

    assert_eq!(
        run_show_with_color(
            &db,
            &result,
            fixed_now(),
            &mut plain,
            &mut Vec::new(),
            false,
        ),
        0
    );
    assert_eq!(
        run_show_with_color(
            &db,
            &result,
            fixed_now(),
            &mut colored,
            &mut Vec::new(),
            true,
        ),
        0
    );

    let plain = String::from_utf8(plain).unwrap();
    let colored = String::from_utf8(colored).unwrap();
    assert!(plain.contains("NAME"));
    assert!(plain.contains("192.168.1.100"));
    assert!(!plain.contains("50001"));
    assert!(plain.contains("100.00Kb"));
    assert!(plain.contains("16m40s"));
    assert!(plain.contains("bob"));
    assert!(colored.contains(&format!("100.00{ANSI_DIM_CYAN}Kb{ANSI_RESET}")));
    assert!(colored.contains(&format!("{ANSI_GREY}never{ANSI_RESET}")));
    assert_eq!(strip_ansi(&colored), plain);
}

fn strip_ansi(value: &str) -> String {
    let mut out = String::new();
    let mut chars = value.chars().peekable();
    while let Some(ch) = chars.next() {
        if ch == '\u{1b}' && chars.peek() == Some(&'[') {
            chars.next();
            for next in chars.by_ref() {
                if ('@'..='~').contains(&next) {
                    break;
                }
            }
            continue;
        }
        out.push(ch);
    }
    out
}
