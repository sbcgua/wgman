use std::collections::{HashMap, HashSet};
use std::fs;

use wgman_rs::config::load_config;
use wgman_rs::db::{
    clone_db, load_db, normalize_access_set, normalize_db_access, save_db_atomic, validate_db,
};
use wgman_rs::model::{
    ResourceEntry, ResourcePort, ResourcePorts, ResourceProtocol, UserEntry, DB,
};

const VALID_DB_YAML: &str = r#"
users:
  admin:
    ip: 10.8.0.5
    pub: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
  alice:
    ip: 10.8.0.10
    pub: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=
vms:
  sandbox: 192.168.122.100
access:
  admin:
    - "*"
  alice:
    - sandbox
"#;

fn write_file(dir: &std::path::Path, name: &str, content: &str) {
    fs::write(dir.join(name), content).unwrap();
}

fn load_testdata_db() -> DB {
    load_db("../testdata/valid-offline").unwrap()
}

#[test]
fn load_config_valid() {
    let dir = tempfile::tempdir().unwrap();
    write_file(
        dir.path(),
        "config.yaml",
        r#"
interface: wg0
sets:
  all: wg_allow_all
  ip_matrix: wg_allow_matrix
  port_matrix: wg_allow_matrix_ports
"#,
    );

    let cfg = load_config(dir.path()).unwrap();
    assert_eq!(cfg.interface, "wg0");
    assert_eq!(cfg.sets.all, "wg_allow_all");
    assert_eq!(cfg.sets.ip_matrix, "wg_allow_matrix");
    assert_eq!(cfg.sets.port_matrix, "wg_allow_matrix_ports");
}

#[test]
fn load_config_from_go_testdata() {
    let cfg = load_config("../testdata/valid-offline").unwrap();
    assert_eq!(cfg.interface, "wg0");
}

#[test]
fn load_config_required_fields_and_unknowns() {
    let cases = [
        (
            "missing interface",
            "sets:\n  all: a\n  ip_matrix: b\n  port_matrix: c\n",
            "interface is required",
        ),
        (
            "missing sets.all",
            "interface: wg0\nsets:\n  ip_matrix: b\n  port_matrix: c\n",
            "sets.all is required",
        ),
        (
            "missing sets.ip_matrix",
            "interface: wg0\nsets:\n  all: a\n  port_matrix: c\n",
            "sets.ip_matrix is required",
        ),
        (
            "missing sets.port_matrix",
            "interface: wg0\nsets:\n  all: a\n  ip_matrix: b\n",
            "sets.port_matrix is required",
        ),
        (
            "unknown field",
            "interface: wg0\nsets:\n  all: a\n  ip_matrix: b\n  port_matrix: c\nextra: bad\n",
            "unknown field",
        ),
    ];

    for (name, yaml, want) in cases {
        let dir = tempfile::tempdir().unwrap();
        write_file(dir.path(), "config.yaml", yaml);
        let err = load_config(dir.path()).expect_err(name);
        assert!(
            err.contains(want),
            "{name}: error {err:?} does not contain {want:?}"
        );
    }
}

#[test]
fn load_config_rejects_yaml_tags() {
    let dir = tempfile::tempdir().unwrap();
    write_file(
        dir.path(),
        "config.yaml",
        "interface: !custom wg0\nsets:\n  all: a\n  ip_matrix: b\n  port_matrix: c\n",
    );
    let err = load_config(dir.path()).expect_err("tagged config scalar");
    assert!(err.contains("YAML tags are not supported"), "{err}");
}

#[test]
fn load_db_valid() {
    let dir = tempfile::tempdir().unwrap();
    write_file(dir.path(), "db.yaml", VALID_DB_YAML);

    let db = load_db(dir.path()).unwrap();
    assert_eq!(db.users.len(), 2);
    assert_eq!(db.vms.len(), 1);
}

#[test]
fn load_db_user_metadata() {
    let dir = tempfile::tempdir().unwrap();
    write_file(
        dir.path(),
        "db.yaml",
        r#"
users:
  alice:
    ip: 10.8.0.10
    pub: ALICE_PUB=
    comment: offboarding pending
    inactive: true
  bob:
    ip: 10.8.0.11
    pub: BOB_PUB=
vms:
  sandbox: 192.168.122.100
"#,
    );

    let db = load_db(dir.path()).unwrap();
    assert_eq!(db.users["alice"].comment, "offboarding pending");
    assert!(db.users["alice"].inactive);
    assert_eq!(db.users["bob"].comment, "");
    assert!(!db.users["bob"].inactive);
}

#[test]
fn load_db_resources() {
    let dir = tempfile::tempdir().unwrap();
    write_file(
        dir.path(),
        "db.yaml",
        r#"
users:
  alice:
    ip: 10.8.0.10
    pub: ALICE_PUB=
vms:
  sandbox: 192.168.122.100
resources:
  ssh@sandbox:
    vm: sandbox
    ports: 22
    comment: shell access
  dns@sandbox:
    vm: sandbox
    ports:
      - udp:53
      - tcp:53
access:
  alice:
    - ssh@sandbox
    - dns@sandbox
"#,
    );

    let db = load_db(dir.path()).unwrap();
    let ssh = &db.resources["ssh@sandbox"];
    assert_eq!(ssh.vm, "sandbox");
    assert_eq!(ssh.comment, "shell access");
    assert_eq!(
        ssh.ports,
        ResourcePorts(vec![ResourcePort {
            protocol: ResourceProtocol::Tcp,
            port: 22,
        }])
    );
    let dns = &db.resources["dns@sandbox"];
    assert_eq!(
        dns.ports,
        ResourcePorts(vec![
            ResourcePort {
                protocol: ResourceProtocol::Udp,
                port: 53,
            },
            ResourcePort {
                protocol: ResourceProtocol::Tcp,
                port: 53,
            },
        ])
    );
}

#[test]
fn load_db_from_go_testdata() {
    load_db("../testdata/valid-offline").unwrap();
}

#[test]
fn load_db_validation_failures() {
    let cases = [
        (
            "missing users section",
            "vms:\n  sandbox: 192.168.122.100\n",
            "users section is required",
        ),
        (
            "missing vms section",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: ABC=\n",
            "vms section is required",
        ),
        (
            "invalid user name",
            "users:\n  \"alice smith\":\n    ip: 10.8.0.10\n    pub: ABC=\nvms:\n  sandbox: 192.168.122.100\n",
            "invalid user name",
        ),
        (
            "user missing ip",
            "users:\n  alice:\n    pub: ABC=\nvms:\n  sandbox: 192.168.122.100\n",
            "ip is required",
        ),
        (
            "user missing pub",
            "users:\n  alice:\n    ip: 10.8.0.10\nvms:\n  sandbox: 192.168.122.100\n",
            "pub is required",
        ),
        (
            "duplicate ips",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n  bob:\n    ip: 10.8.0.10\n    pub: BBBB=\nvms:\n  sandbox: 192.168.122.100\n",
            "duplicate ip",
        ),
        (
            "duplicate pub keys",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n  bob:\n    ip: 10.8.0.11\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\n",
            "duplicate pub key",
        ),
        (
            "case-conflicting user names",
            "users:\n  Alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n  alice:\n    ip: 10.8.0.11\n    pub: BBBB=\nvms:\n  sandbox: 192.168.122.100\n",
            "conflicts with",
        ),
        (
            "invalid vm name",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  \"sand box\": 192.168.122.100\n",
            "invalid vm name",
        ),
        (
            "access references unknown target",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  alice:\n    - unknownvm\n",
            "unknown access target",
        ),
        (
            "resource references unknown vm",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nresources:\n  ssh@sandbox:\n    vm: missing\n    ports: 22\n",
            "unknown vm",
        ),
        (
            "resource conflicts with vm",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nresources:\n  sandbox:\n    vm: sandbox\n    ports: 22\n",
            "conflicts",
        ),
        (
            "resource duplicate normalized port",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nresources:\n  ssh@sandbox:\n    vm: sandbox\n    ports: [22, tcp:22]\n",
            "duplicate port",
        ),
        (
            "vm name cannot contain at",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  ssh@sandbox: 192.168.122.100\n",
            "invalid vm name",
        ),
        (
            "access references unknown user",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  nobody:\n    - sandbox\n",
            "unknown user",
        ),
        (
            "duplicate access entry",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\naccess:\n  alice:\n    - sandbox\n    - sandbox\n",
            "duplicate entry",
        ),
        (
            "unknown field in db",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\nextrafield: bad\n",
            "unknown field",
        ),
        (
            "unknown field in user",
            "users:\n  alice:\n    ip: 10.8.0.10\n    pub: AAAA=\n    extra: bad\nvms:\n  sandbox: 192.168.122.100\n",
            "unknown field",
        ),
    ];

    for (name, yaml, want) in cases {
        let dir = tempfile::tempdir().unwrap();
        write_file(dir.path(), "db.yaml", yaml);
        let err = load_db(dir.path()).expect_err(name);
        assert!(
            err.contains(want),
            "{name}: error {err:?} does not contain {want:?}"
        );
    }
}

#[test]
fn load_db_rejects_yaml_tags() {
    let dir = tempfile::tempdir().unwrap();
    write_file(
        dir.path(),
        "db.yaml",
        "users:\n  alice:\n    ip: !custom 10.8.0.10\n    pub: AAAA=\nvms:\n  sandbox: 192.168.122.100\n",
    );
    let err = load_db(dir.path()).expect_err("tagged db scalar");
    assert!(err.contains("YAML tags are not supported"), "{err}");
}

#[test]
fn validate_db_rejects_invalid_ips_and_mixed_star() {
    let mut db = load_testdata_db();
    db.users.get_mut("alice").unwrap().ip = "not-an-ip".to_string();
    assert!(validate_db(&db)
        .iter()
        .any(|err| err.contains("invalid ip")));

    let mut db = load_testdata_db();
    db.users.get_mut("alice").unwrap().ip = "::1".to_string();
    assert!(validate_db(&db)
        .iter()
        .any(|err| err.contains("invalid ip")));

    let mut db = load_testdata_db();
    db.vms
        .insert("sandbox".to_string(), "2001:db8::1".to_string());
    assert!(validate_db(&db)
        .iter()
        .any(|err| err.contains("invalid ip")));

    let mut db = load_testdata_db();
    db.access.insert(
        "alice".to_string(),
        vec!["*".to_string(), "sandbox".to_string()],
    );
    assert!(validate_db(&db)
        .iter()
        .any(|err| err.contains("\"*\" must be the sole entry")));
}

#[test]
fn save_db_atomic_deterministic_output() {
    let dir = tempfile::tempdir().unwrap();
    let db = DB {
        users: HashMap::from([
            (
                "bob".to_string(),
                UserEntry {
                    ip: "10.8.0.15".to_string(),
                    pub_key: "BOB_PUB=".to_string(),
                    comment: "temporary contractor".to_string(),
                    inactive: true,
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
        ]),
        vms: HashMap::from([
            ("sandbox".to_string(), "192.168.122.100".to_string()),
            ("mailvm".to_string(), "192.168.122.101".to_string()),
        ]),
        resources: HashMap::new(),
        access: HashMap::from([
            (
                "bob".to_string(),
                vec!["sandbox".to_string(), "mailvm".to_string()],
            ),
            ("alice".to_string(), vec!["sandbox".to_string()]),
        ]),
    };

    save_db_atomic(dir.path(), &db).unwrap();
    let data = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    let want = "users:\n  alice:\n    ip: 10.8.0.10\n    pub: ALICE_PUB=\n  bob:\n    ip: 10.8.0.15\n    pub: BOB_PUB=\n    comment: temporary contractor\n    inactive: true\n\nvms:\n  mailvm: 192.168.122.101\n  sandbox: 192.168.122.100\n\nresources: {}\n\naccess:\n  alice:\n    - sandbox\n  bob:\n    - sandbox\n    - mailvm\n";
    assert_eq!(data, want);

    let loaded = load_db(dir.path()).unwrap();
    assert_eq!(loaded.users["alice"].comment, "");
    assert!(!loaded.users["alice"].inactive);
    assert_eq!(loaded.users["bob"].comment, "temporary contractor");
    assert!(loaded.users["bob"].inactive);
}

#[cfg(unix)]
#[test]
fn save_db_atomic_uses_private_permissions() {
    use std::os::unix::fs::PermissionsExt;

    let dir = tempfile::tempdir().unwrap();
    let db = load_testdata_db();
    save_db_atomic(dir.path(), &db).unwrap();
    let mode = fs::metadata(dir.path().join("db.yaml"))
        .unwrap()
        .permissions()
        .mode()
        & 0o777;
    assert_eq!(mode, 0o600);
}

#[test]
fn save_db_atomic_quotes_yaml_ambiguous_strings_for_round_trip() {
    let dir = tempfile::tempdir().unwrap();
    let db = DB {
        users: HashMap::from([(
            "123".to_string(),
            UserEntry {
                ip: "10.8.0.10".to_string(),
                pub_key: "456".to_string(),
                comment: "true".to_string(),
                inactive: false,
            },
        )]),
        vms: HashMap::from([("789".to_string(), "192.168.122.100".to_string())]),
        resources: HashMap::new(),
        access: HashMap::from([("123".to_string(), vec!["789".to_string()])]),
    };

    save_db_atomic(dir.path(), &db).unwrap();
    let data = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(data.contains("  \"123\":\n"), "{data}");
    assert!(data.contains("    pub: \"456\"\n"), "{data}");
    assert!(data.contains("    comment: \"true\"\n"), "{data}");
    assert!(data.contains("  \"789\": 192.168.122.100\n"), "{data}");
    assert!(data.contains("    - \"789\"\n"), "{data}");

    let loaded = load_db(dir.path()).unwrap();
    assert_eq!(loaded, db);
}

#[test]
fn save_db_atomic_writes_empty_users_as_mapping() {
    let dir = tempfile::tempdir().unwrap();
    let db = DB {
        users: HashMap::new(),
        vms: HashMap::from([("sandbox".to_string(), "192.168.122.100".to_string())]),
        resources: HashMap::new(),
        access: HashMap::new(),
    };

    save_db_atomic(dir.path(), &db).unwrap();
    let data = fs::read_to_string(dir.path().join("db.yaml")).unwrap();
    assert!(data.starts_with("users: {}\n\n"), "{data}");

    let loaded = load_db(dir.path()).unwrap();
    assert_eq!(loaded, db);
}

#[test]
fn clone_db_is_independent() {
    let original = DB {
        users: HashMap::from([(
            "alice".to_string(),
            UserEntry {
                ip: "10.8.0.10".to_string(),
                pub_key: "ALICE".to_string(),
                comment: String::new(),
                inactive: false,
            },
        )]),
        vms: HashMap::from([("sandbox".to_string(), "192.168.122.100".to_string())]),
        resources: HashMap::from([(
            "ssh@sandbox".to_string(),
            ResourceEntry {
                vm: "sandbox".to_string(),
                ports: ResourcePorts(vec![ResourcePort {
                    protocol: ResourceProtocol::Tcp,
                    port: 22,
                }]),
                comment: String::new(),
            },
        )]),
        access: HashMap::from([("alice".to_string(), vec!["sandbox".to_string()])]),
    };

    let mut cloned = clone_db(&original);
    cloned.users.insert(
        "alice".to_string(),
        UserEntry {
            ip: "10.8.0.20".to_string(),
            pub_key: "CHANGED".to_string(),
            comment: String::new(),
            inactive: false,
        },
    );
    cloned
        .vms
        .insert("sandbox".to_string(), "192.168.122.200".to_string());
    cloned.resources.insert(
        "ssh@sandbox".to_string(),
        ResourceEntry {
            vm: "sandbox".to_string(),
            ports: ResourcePorts(vec![ResourcePort {
                protocol: ResourceProtocol::Tcp,
                port: 2222,
            }]),
            comment: String::new(),
        },
    );
    cloned.access.get_mut("alice").unwrap()[0] = "changed".to_string();

    assert_eq!(original.users["alice"].ip, "10.8.0.10");
    assert_eq!(original.vms["sandbox"], "192.168.122.100");
    assert_eq!(original.resources["ssh@sandbox"].ports.0[0].port, 22);
    assert_eq!(original.access["alice"][0], "sandbox");
}

#[test]
fn normalize_access_helpers() {
    let mut db = DB {
        access: HashMap::from([
            (
                "alice".to_string(),
                vec![
                    "sandbox".to_string(),
                    "mail".to_string(),
                    "sandbox".to_string(),
                ],
            ),
            ("bob".to_string(), Vec::new()),
        ]),
        ..DB::default()
    };

    normalize_db_access(&mut db);
    assert_eq!(
        db.access,
        HashMap::from([(
            "alice".to_string(),
            vec!["mail".to_string(), "sandbox".to_string()]
        )])
    );

    let empty: HashSet<String> = HashSet::new();
    assert!(normalize_access_set(&empty).is_empty());
    let set = HashSet::from(["sandbox".to_string(), "mail".to_string()]);
    assert_eq!(
        normalize_access_set(&set),
        vec!["mail".to_string(), "sandbox".to_string()]
    );
}
