mod support;

use std::collections::HashMap;
use std::ffi::OsString;
use std::fs;
use std::io::Cursor;
#[cfg(unix)]
use std::os::unix::fs::PermissionsExt;
use std::sync::{Mutex, OnceLock};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use support::FakeSystem;
use tempfile::TempDir;
use wgman_rs::cli::{App, GlobalFlags};
use wgman_rs::client_config::{render_client_config, write_client_config_no_overwrite};
use wgman_rs::commands::create::cmd_create;

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

fn cwd_lock() -> &'static Mutex<()> {
    static LOCK: OnceLock<Mutex<()>> = OnceLock::new();
    LOCK.get_or_init(|| Mutex::new(()))
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
    fs::write(
        dir.path().join("user.conf.template"),
        concat!(
            "# generated\n",
            "\n",
            "[Interface]\n",
            "PrivateKey = $CLIENT_PRIVATE_KEY\n",
            "Address = $CLIENT_VPN_IP/32\n",
            "\n",
            "[Peer]\n",
            "PublicKey = $SERVER_PUBLIC_KEY\n",
        ),
    )
    .expect("write template");
    dir
}

fn write_inactive_bob_config_dir() -> TempDir {
    let dir = write_config_dir();
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
    .expect("write inactive db");
    dir
}

fn write_resource_config_dir() -> TempDir {
    let dir = write_config_dir();
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
    .expect("write resource db");
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
        ipset_results: base_ipsets(),
        ..FakeSystem::default()
    }
}

fn resource_clean_system() -> FakeSystem {
    clean_system()
}

fn inactive_bob_absent_system() -> FakeSystem {
    FakeSystem {
        root: true,
        subnet_result: "10.8.0.1/24".to_string(),
        wg_dump_result: concat!(
            "SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n",
            "ADMIN_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\toff\n",
            "ALICE_PUB=\t(none)\t(none)\t10.8.0.10/32\t0\t0\t0\toff\n",
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

#[cfg(unix)]
fn make_config_dir_read_only(dir: &TempDir) {
    fs::set_permissions(dir.path(), fs::Permissions::from_mode(0o500))
        .expect("chmod config dir read-only");
}

#[cfg(unix)]
fn make_config_dir_writable(dir: &TempDir) {
    fs::set_permissions(dir.path(), fs::Permissions::from_mode(0o700))
        .expect("chmod config dir writable");
}

fn drift_system() -> FakeSystem {
    let mut system = clean_system();
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

fn base_ipsets() -> HashMap<String, String> {
    HashMap::from([
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
    ])
}

#[test]
fn client_config_renders_template_and_write_does_not_overwrite() {
    let dir = tempfile::tempdir().expect("tempdir");
    let template = dir.path().join("user.conf.template");
    fs::write(
        &template,
        "# comment\n  # local note\n\npriv=$CLIENT_PRIVATE_KEY\nip=$CLIENT_VPN_IP\nserver=$SERVER_PUBLIC_KEY\n",
    )
    .expect("write template");

    let rendered = render_client_config(&template, "PRIV", "10.8.0.20", "SERVER").unwrap();
    assert_eq!(rendered, "priv=PRIV\nip=10.8.0.20\nserver=SERVER\n");

    let path = dir.path().join("alice.vpn.conf");
    write_client_config_no_overwrite(&path, "first").unwrap();
    assert!(write_client_config_no_overwrite(&path, "second").is_err());
    assert_eq!(fs::read_to_string(path).unwrap(), "first");
}

#[test]
fn create_adds_user_writes_config_without_private_key_in_db() {
    let _guard = cwd_lock().lock().unwrap();
    let original = std::env::current_dir().unwrap();
    let dir = write_config_dir();
    let out_dir = tempfile::tempdir().expect("out dir");
    std::env::set_current_dir(out_dir.path()).unwrap();

    let dir_string = dir.path().display().to_string();
    let mut system = clean_system();
    system.gen_key_result = "CAROL_PRIVATE".to_string();
    system.pub_key_result = "CAROL_PUBLIC=".to_string();
    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "create",
            "--config-dir",
            &dir_string,
            "-c= laptop ",
            "carol",
            "mailvm",
        ],
        system,
        "",
    );
    std::env::set_current_dir(original).unwrap();

    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("created carol"));
    let db_text = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(db_text.contains("carol:\n    ip: 10.8.0.16\n    pub: CAROL_PUBLIC=\n"));
    assert!(db_text.contains("comment: laptop"));
    assert!(!db_text.contains("CAROL_PRIVATE"));
    let config_text = fs::read_to_string(out_dir.path().join("carol.vpn.conf")).unwrap();
    assert!(config_text.contains("PrivateKey = CAROL_PRIVATE"));
    assert!(config_text.contains("Address = 10.8.0.16/32"));
    assert!(config_text.contains("PublicKey = SERVER_PUB="));
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "wgset:wg0:CAROL_PUBLIC=:10.8.0.16",
            "add:wg_allow_matrix:10.8.0.16,192.168.122.101:carol -> mailvm",
        ]
    );
}

#[test]
fn create_rejects_duplicate_case_ip_pub_and_adds_resource_port_access() {
    let _guard = cwd_lock().lock().unwrap();
    let original = std::env::current_dir().unwrap();
    let dir = write_resource_config_dir();
    let out_dir = tempfile::tempdir().expect("out dir");
    std::env::set_current_dir(out_dir.path()).unwrap();
    let dir_string = dir.path().display().to_string();

    for (name, want) in [("alice", "already exists"), ("Alice", "conflicts")] {
        let (code, _, stderr, _) = run(
            &["wgman-rs", "create", "--config-dir", &dir_string, name],
            resource_clean_system(),
            "",
        );
        assert_eq!(code, 1);
        assert!(stderr.contains(want), "{stderr}");
    }

    let (code, _, stderr, _) = run(
        &[
            "wgman-rs",
            "create",
            "--config-dir",
            &dir_string,
            "carol",
            "10.8.0.10",
        ],
        resource_clean_system(),
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("conflicts"), "{stderr}");

    let mut system = resource_clean_system();
    system.pub_key_result = "ALICE_PUB=".to_string();
    let (code, _, stderr, _) = run(
        &[
            "wgman-rs",
            "create",
            "--config-dir",
            &dir_string,
            "carol",
            "10.8.0.20",
        ],
        system,
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("public key conflicts"), "{stderr}");

    let mut system = resource_clean_system();
    system.gen_key_result = "CAROL_PRIVATE".to_string();
    system.pub_key_result = "CAROL_PUBLIC=".to_string();
    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "create",
            "--config-dir",
            &dir_string,
            "carol",
            "ssh@sandbox",
        ],
        system,
        "",
    );
    std::env::set_current_dir(original).unwrap();

    assert_eq!(code, 0, "{stderr}");
    assert!(system.applied_ops.borrow().contains(
        &"add:wg_allow_matrix_ports:10.8.0.16,tcp:22,192.168.122.100:carol -> ssh@sandbox tcp/22"
            .to_string()
    ));
}

#[test]
fn create_rejects_dry_run_drift_bad_args_and_rolls_back_live_and_config() {
    let _guard = cwd_lock().lock().unwrap();
    let original = std::env::current_dir().unwrap();
    let dir = write_config_dir();
    let out_dir = tempfile::tempdir().expect("out dir");
    std::env::set_current_dir(out_dir.path()).unwrap();
    let dir_string = dir.path().display().to_string();

    let (code, _, stderr, _) = run(
        &["wgman-rs", "create", "--dry-run", "carol"],
        FakeSystem::default(),
        "",
    );
    assert_eq!(code, 2);
    assert!(stderr.contains("does not support --dry-run"));

    let app = App::new(FakeSystem::default());
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();
    let code = cmd_create(
        &GlobalFlags {
            dry_run: true,
            ..GlobalFlags::default()
        },
        &[String::from("carol")],
        &app,
        &mut stdout,
        &mut stderr,
    );
    assert_eq!(code, 2);
    assert!(String::from_utf8(stderr)
        .unwrap()
        .contains("does not support --dry-run"));

    let (code, _, stderr, _) = run(
        &[
            "wgman-rs",
            "create",
            "--config-dir",
            &dir_string,
            "carol",
            "10.8.0.0/24",
            "sandbox",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 2);
    assert!(stderr.contains("not a valid IPv4"));

    let (code, _, stderr, _) = run(
        &["wgman-rs", "create", "--config-dir", &dir_string, "carol"],
        drift_system(),
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("ipset drift"));

    let before = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    let mut system = clean_system();
    system.gen_key_result = "CAROL_PRIVATE".to_string();
    system.pub_key_result = "CAROL_PUBLIC=".to_string();
    system.ipset_add_error = Some("permission denied".to_string());
    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "create",
            "--config-dir",
            &dir_string,
            "carol",
            "mailvm",
        ],
        system,
        "",
    );
    std::env::set_current_dir(original).unwrap();

    assert_eq!(code, 1);
    assert!(stderr.contains("permission denied"));
    assert_eq!(
        fs::read_to_string(dir.path().join("db.yaml")).unwrap(),
        before
    );
    assert!(!out_dir.path().join("carol.vpn.conf").exists());
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "wgset:wg0:CAROL_PUBLIC=:10.8.0.16",
            "wgdel:wg0:CAROL_PUBLIC=",
        ]
    );
}

#[cfg(unix)]
#[test]
fn create_db_save_failure_rolls_back_live_state_and_generated_config() {
    let _guard = cwd_lock().lock().unwrap();
    let original = std::env::current_dir().unwrap();
    let dir = write_config_dir();
    let out_dir = tempfile::tempdir().expect("out dir");
    std::env::set_current_dir(out_dir.path()).unwrap();
    let before = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    let dir_string = dir.path().display().to_string();
    make_config_dir_read_only(&dir);

    let mut system = clean_system();
    system.gen_key_result = "CAROL_PRIVATE".to_string();
    system.pub_key_result = "CAROL_PUBLIC=".to_string();
    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "create",
            "--config-dir",
            &dir_string,
            "carol",
            "mailvm",
        ],
        system,
        "",
    );
    make_config_dir_writable(&dir);
    std::env::set_current_dir(original).unwrap();

    assert_eq!(code, 1);
    assert!(
        stderr.contains("create temporary db") || stderr.contains("Permission denied"),
        "{stderr}"
    );
    assert_eq!(
        fs::read_to_string(dir.path().join("db.yaml")).unwrap(),
        before
    );
    assert!(!out_dir.path().join("carol.vpn.conf").exists());
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        [
            "wgset:wg0:CAROL_PUBLIC=:10.8.0.16",
            "add:wg_allow_matrix:10.8.0.16,192.168.122.101:carol -> mailvm",
            "del:wg_allow_matrix:10.8.0.16,192.168.122.101",
            "wgdel:wg0:CAROL_PUBLIC=",
        ]
    );
}

#[test]
fn remove_supports_dry_run_prompt_apply_and_rollback() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();
    let before = fs::read_to_string(dir.path().join("db.yaml")).unwrap();

    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "remove",
            "--config-dir",
            &dir_string,
            "--dry-run",
            "alice",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("planned removal"));
    assert!(stdout.contains("dry-run"));
    assert_eq!(
        fs::read_to_string(dir.path().join("db.yaml")).unwrap(),
        before
    );
    assert!(system.applied_ops.borrow().is_empty());

    let (code, stdout, _, system) = run(
        &["wgman-rs", "remove", "--config-dir", &dir_string, "alice"],
        clean_system(),
        "n\n",
    );
    assert_eq!(code, 0);
    assert!(stdout.contains("aborted"));
    assert!(system.applied_ops.borrow().is_empty());

    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "remove",
            "--config-dir",
            &dir_string,
            "--yes",
            "alice",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("removed alice"));
    assert!(system
        .applied_ops
        .borrow()
        .contains(&"wgdel:wg0:ALICE_PUB=".to_string()));
    let db_text = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(!db_text.contains("alice:\n    ip:"));

    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();
    let before = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    let mut system = clean_system();
    system.wg_del_error = Some("permission denied".to_string());
    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "remove",
            "--config-dir",
            &dir_string,
            "--yes",
            "bob",
        ],
        system,
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("permission denied"));
    assert_eq!(
        fs::read_to_string(dir.path().join("db.yaml")).unwrap(),
        before
    );
    let ops = system.applied_ops.borrow();
    assert!(ops.contains(&"del:wg_allow_matrix:10.8.0.15,192.168.122.100".to_string()));
    assert!(
        ops.contains(&"add:wg_allow_matrix:10.8.0.15,192.168.122.100:bob -> sandbox".to_string())
    );
}

#[test]
fn remove_rejects_missing_user_and_admin_delete_removes_all_access() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, _, stderr, _) = run(
        &[
            "wgman-rs",
            "remove",
            "--config-dir",
            &dir_string,
            "--yes",
            "nobody",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("not found"), "{stderr}");

    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "remove",
            "--config-dir",
            &dir_string,
            "--yes",
            "admin",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(system
        .applied_ops
        .borrow()
        .contains(&"del:wg_allow_all:10.8.0.5".to_string()));
}

#[cfg(unix)]
#[test]
fn remove_db_save_failure_rolls_back_live_state() {
    let dir = write_config_dir();
    let before = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    let dir_string = dir.path().display().to_string();
    make_config_dir_read_only(&dir);

    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "remove",
            "--config-dir",
            &dir_string,
            "--yes",
            "bob",
        ],
        clean_system(),
        "",
    );
    make_config_dir_writable(&dir);

    assert_eq!(code, 1);
    assert!(
        stderr.contains("create temporary db") || stderr.contains("Permission denied"),
        "{stderr}"
    );
    assert_eq!(
        fs::read_to_string(dir.path().join("db.yaml")).unwrap(),
        before
    );
    let ops = system.applied_ops.borrow();
    for want in [
        "del:wg_allow_matrix:10.8.0.15,192.168.122.100",
        "del:wg_allow_matrix:10.8.0.15,192.168.122.101",
        "wgdel:wg0:BOB_PUB=",
        "wgset:wg0:BOB_PUB=:10.8.0.15",
        "add:wg_allow_matrix:10.8.0.15,192.168.122.101:bob -> mailvm",
        "add:wg_allow_matrix:10.8.0.15,192.168.122.100:bob -> sandbox",
    ] {
        assert!(
            ops.contains(&want.to_string()),
            "missing {want} from {ops:?}"
        );
    }
}

#[test]
fn mod_access_and_activation_paths_support_dry_run_apply_and_rollback() {
    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();

    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "--dry-run",
            "alice",
            "+mailvm",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("dry-run"));
    assert!(system.applied_ops.borrow().is_empty());

    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "alice",
            "+mailvm",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("applied"));
    assert!(system
        .applied_ops
        .borrow()
        .contains(&"add:wg_allow_matrix:10.8.0.10,192.168.122.101:alice -> mailvm".to_string()));
    let db_text = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(db_text.contains("alice:\n    - mailvm\n    - sandbox\n"));

    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();
    let mut system = clean_system();
    system.ipset_add_errors = HashMap::from([(
        "wg_allow_matrix:10.8.0.10,192.168.122.101".to_string(),
        "permission denied".to_string(),
    )]);
    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "alice",
            "+mailvm,-sandbox",
        ],
        system,
        "",
    );
    assert_eq!(code, 1);
    assert!(stderr.contains("permission denied"));
    let db_text = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(db_text.contains("alice:\n    - mailvm\n"));
    assert!(!db_text.contains("alice:\n    - sandbox\n"));
    assert_eq!(
        system.applied_ops.borrow().as_slice(),
        ["del:wg_allow_matrix:10.8.0.10,192.168.122.100"]
    );

    let dir = write_inactive_bob_config_dir();
    let dir_string = dir.path().display().to_string();
    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "bob",
            "activate",
        ],
        inactive_bob_absent_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("applied"));
    let ops = system.applied_ops.borrow();
    assert!(ops.contains(&"wgset:wg0:BOB_PUB=:10.8.0.15".to_string()));
    assert!(
        ops.contains(&"add:wg_allow_matrix:10.8.0.15,192.168.122.100:bob -> sandbox".to_string())
    );
    let db_text = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(!db_text.contains("inactive: true"));

    let (code, _, stderr, _) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "bob",
            "enable",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 2);
    assert!(stderr.contains("must start with + or -"));
}

#[test]
fn mod_resource_access_inactive_db_only_and_redundant_toggles() {
    let dir = write_resource_config_dir();
    let dir_string = dir.path().display().to_string();
    let (code, _, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "alice",
            "+ssh@sandbox",
        ],
        resource_clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(system.applied_ops.borrow().contains(
        &"add:wg_allow_matrix_ports:10.8.0.10,tcp:22,192.168.122.100:alice -> ssh@sandbox tcp/22"
            .to_string()
    ));

    let dir = write_inactive_bob_config_dir();
    let dir_string = dir.path().display().to_string();
    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "bob",
            "-mailvm",
        ],
        inactive_bob_absent_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("update db access for bob"));
    assert!(system.applied_ops.borrow().is_empty());
    let db_text = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(db_text.contains("bob:\n    - sandbox\n"));
    assert!(!db_text.contains("bob:\n    - mailvm\n"));
    assert!(db_text.contains("inactive: true"));

    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "bob",
            "deactivate",
        ],
        inactive_bob_absent_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("already inactive"));
    assert!(system.applied_ops.borrow().is_empty());

    let dir = write_config_dir();
    let dir_string = dir.path().display().to_string();
    let (code, stdout, stderr, system) = run(
        &[
            "wgman-rs",
            "mod",
            "--config-dir",
            &dir_string,
            "alice",
            "activate",
        ],
        clean_system(),
        "",
    );
    assert_eq!(code, 0, "{stderr}");
    assert!(stdout.contains("already active"));
    assert!(system.applied_ops.borrow().is_empty());
}
