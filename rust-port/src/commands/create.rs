use std::collections::HashSet;
use std::fs;
use std::io::Write;
use std::net::Ipv4Addr;
use std::path::Path;

use crate::check::{check, compute_expected_ipsets};
use crate::cli::{App, GlobalFlags};
use crate::client_config::{render_client_config, write_client_config_no_overwrite};
use crate::config::load_config;
use crate::db::{clone_db, normalize_db_access, save_db_atomic, validate_db};
use crate::deploy::{apply_state_deltas_tracked, diff_expected_ipsets, rollback_state_deltas};
use crate::model::{Config, IPSetDelta, UserEntry, WGPeerDelta, WGPeerDeltaAction, DB};
use crate::output::{print_apply_and_rollback_error, print_check_errors, print_ipset_deltas};
use crate::system::SystemAdapter;
use crate::utils::{case_fold_ascii, ipv4_to_u32, parse_ipv4_cidr, uint32_to_ipv4};

#[derive(Clone, Debug, PartialEq, Eq)]
struct CreateArgs {
    name: String,
    ip: String,
    access: Vec<String>,
    comment: String,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct CreatePlan {
    updated_db: DB,
    ipset_deltas: Vec<IPSetDelta>,
    peer_deltas: Vec<WGPeerDelta>,
    client_ip: String,
    private_key: String,
}

pub fn cmd_create<S, W, E>(
    flags: &GlobalFlags,
    args: &[String],
    app: &App<S>,
    stdout: &mut W,
    stderr: &mut E,
) -> i32
where
    S: SystemAdapter,
    W: Write,
    E: Write,
{
    if !app.system().is_root() {
        let _ = writeln!(stderr, "error: wgman must be run as root");
        return 1;
    }

    let parsed = match parse_create_args(args, &flags.create_comment, flags.create_comment_set) {
        Ok(parsed) => parsed,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 2;
        }
    };

    let cfg = match load_config(&flags.config_dir) {
        Ok(cfg) => cfg,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };
    let db = match crate::db::load_db(&flags.config_dir) {
        Ok(db) => db,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };

    let result = check(&cfg, &db, app.system());
    if !result.ok() {
        print_check_errors(&result, stderr);
        let _ = writeln!(
            stderr,
            "create: FAILED (resolve hard errors or drift before creating users)"
        );
        return 1;
    }
    let Some(wg_dump) = &result.wg_dump else {
        let _ = writeln!(stderr, "create: no WireGuard data available");
        return 1;
    };

    let client_config_path = format!("{}.vpn.conf", parsed.name);
    match Path::new(&client_config_path).try_exists() {
        Ok(true) => {
            let _ = writeln!(stderr, "error: {client_config_path} already exists");
            return 1;
        }
        Ok(false) => {}
        Err(err) => {
            let _ = writeln!(stderr, "error: check {client_config_path}: {err}");
            return 1;
        }
    }

    let plan = match plan_create_user(&cfg, &db, &parsed, app.system()) {
        Ok(plan) => plan,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };

    let template_path = Path::new(&flags.config_dir).join("user.conf.template");
    let client_config = match render_client_config(
        template_path,
        &plan.private_key,
        &plan.client_ip,
        &wg_dump.server_pub_key,
    ) {
        Ok(config) => config,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };

    let _ = writeln!(
        stdout,
        "create: planned user {} at {}",
        parsed.name, plan.client_ip
    );
    if !plan.ipset_deltas.is_empty() {
        let _ = writeln!(
            stdout,
            "create: planned access changes ({}):",
            plan.ipset_deltas.len()
        );
        print_ipset_deltas(&plan.ipset_deltas, stdout);
    }

    if let Err(err) = write_client_config_no_overwrite(&client_config_path, &client_config) {
        let _ = writeln!(stderr, "error: {err}");
        return 1;
    }

    if let Err((err, rollback_err)) = apply_create_user_plan(
        &flags.config_dir,
        &cfg.interface,
        &client_config_path,
        &plan,
        app.system(),
    ) {
        print_apply_and_rollback_error(&err, rollback_err.as_deref(), stderr);
        return 1;
    }

    let _ = writeln!(stdout, "create: created {}", parsed.name);
    0
}

fn apply_create_user_plan<S: SystemAdapter>(
    config_dir: &str,
    iface: &str,
    client_config_path: &str,
    plan: &CreatePlan,
    system: &S,
) -> Result<(), (String, Option<String>)> {
    let applied =
        match apply_state_deltas_tracked(iface, &plan.ipset_deltas, &plan.peer_deltas, system) {
            Ok(applied) => applied,
            Err(err) => {
                let rollback_err =
                    rollback_create_live_state(iface, client_config_path, &err.applied, system);
                return Err((err.error, rollback_err));
            }
        };

    if let Err(err) = save_db_atomic(config_dir, &plan.updated_db) {
        let rollback_err = rollback_create_live_state(iface, client_config_path, &applied, system);
        return Err((err, rollback_err));
    }

    Ok(())
}

fn rollback_create_live_state<S: SystemAdapter>(
    iface: &str,
    client_config_path: &str,
    applied: &crate::deploy::AppliedStateDeltas,
    system: &S,
) -> Option<String> {
    let mut errors = Vec::new();
    if let Err(err) = rollback_state_deltas(iface, applied, system) {
        errors.push(err);
    }
    if let Err(err) = fs::remove_file(client_config_path) {
        if err.kind() != std::io::ErrorKind::NotFound {
            errors.push(err.to_string());
        }
    }
    (!errors.is_empty()).then(|| errors.join("; "))
}

fn plan_create_user<S: SystemAdapter>(
    cfg: &Config,
    db: &DB,
    args: &CreateArgs,
    system: &S,
) -> Result<CreatePlan, String> {
    if !valid_user_name(&args.name) {
        return Err(format!("invalid user name {:?}", args.name));
    }
    if db.users.contains_key(&args.name) {
        return Err(format!("user {:?} already exists", args.name));
    }
    for existing in db.users.keys() {
        if case_fold_ascii(existing) == case_fold_ascii(&args.name) {
            return Err(format!(
                "user name {:?} conflicts with {:?} (case)",
                args.name, existing
            ));
        }
    }

    let subnet = system.interface_subnet(&cfg.interface)?;
    let private_key = system.wg_gen_key()?;
    let pub_key = system.wg_pub_key(&private_key)?;
    if pub_key.is_empty() {
        return Err("generated public key is empty".to_string());
    }
    for (existing, user) in &db.users {
        if user.pub_key == pub_key {
            return Err(format!(
                "generated public key conflicts with existing user {:?}",
                existing
            ));
        }
    }

    let client_ip = if args.ip.is_empty() {
        allocate_next_user_ip(&subnet, db)?
    } else {
        args.ip.clone()
    };
    let client_ip_addr: Ipv4Addr = client_ip
        .parse()
        .map_err(|_| format!("invalid client IP {client_ip:?}"))?;
    let parsed_subnet = parse_ipv4_cidr(&subnet)?;
    if !ipv4_cidr_contains(&parsed_subnet, client_ip_addr) {
        return Err(format!(
            "client IP {client_ip} is outside interface subnet {subnet}"
        ));
    }
    for (existing, user) in &db.users {
        if user.ip == client_ip {
            return Err(format!(
                "client IP {client_ip} conflicts with existing user {:?}",
                existing
            ));
        }
    }

    let mut updated = clone_db(db);
    updated.users.insert(
        args.name.clone(),
        UserEntry {
            ip: client_ip.clone(),
            pub_key: pub_key.clone(),
            comment: args.comment.clone(),
            inactive: false,
        },
    );
    if !args.access.is_empty() {
        updated
            .access
            .insert(args.name.clone(), args.access.clone());
    }
    normalize_db_access(&mut updated);
    let errs = validate_db(&updated);
    if !errs.is_empty() {
        return Err(format!(
            "updated db.yaml would be invalid: {}",
            errs.join("; ")
        ));
    }

    let old_expected = compute_expected_ipsets(db);
    let new_expected = compute_expected_ipsets(&updated);
    Ok(CreatePlan {
        updated_db: updated,
        ipset_deltas: diff_expected_ipsets(cfg, &old_expected, &new_expected),
        peer_deltas: vec![WGPeerDelta {
            user: args.name.clone(),
            pub_key,
            allowed_ip: client_ip.clone(),
            action: WGPeerDeltaAction::Add,
        }],
        client_ip,
        private_key,
    })
}

fn allocate_next_user_ip(subnet: &str, db: &DB) -> Result<String, String> {
    let cidr = parse_ipv4_cidr(subnet)?;
    let network = u64::from(ipv4_to_u32(cidr.network));
    let size = 1_u64 << u32::from(32 - cidr.prefix);
    if size <= 2 {
        return Err(format!(
            "interface subnet {subnet:?} has no usable client addresses"
        ));
    }

    let first = network + 1;
    let last = network + size - 2;
    let mut max_used = u64::from(ipv4_to_u32(cidr.ip));
    if max_used < first {
        max_used = first - 1;
    }
    for user in db.users.values() {
        let Ok(ip) = user.ip.parse::<Ipv4Addr>() else {
            continue;
        };
        if !ipv4_cidr_contains(&cidr, ip) {
            continue;
        }
        max_used = max_used.max(u64::from(ipv4_to_u32(ip)));
    }
    let next = max_used + 1;
    if next > last {
        return Err(format!("no available IP in {subnet} after existing users"));
    }
    Ok(uint32_to_ipv4(next as u32).to_string())
}

fn parse_create_args(
    args: &[String],
    comment: &str,
    comment_set: bool,
) -> Result<CreateArgs, String> {
    let (args, mut comment, comment_set) =
        extract_create_comment_flags(args, comment.to_string(), comment_set)?;
    if comment_set {
        comment = comment.trim().to_string();
        if comment.is_empty() {
            return Err("create comment must not be empty".to_string());
        }
    }
    if args.is_empty() || args.len() > 3 {
        return Err("create requires <name> [ip] [res1,res2...]".to_string());
    }
    if !valid_user_name(&args[0]) {
        return Err(format!("invalid user name {:?}", args[0]));
    }

    let mut parsed = CreateArgs {
        name: args[0].clone(),
        ip: String::new(),
        access: Vec::new(),
        comment,
    };
    if args.len() == 1 {
        return Ok(parsed);
    }
    if let Some(ip) = parse_create_ip_arg(&args[1]) {
        parsed.ip = ip;
        if args.len() == 3 {
            parsed.access = parse_create_access_list(&args[2])?;
        }
        return Ok(parsed);
    }
    if args.len() == 3 {
        return Err(format!(
            "second argument {:?} is not a valid IPv4 address or CIDR",
            args[1]
        ));
    }
    parsed.access = parse_create_access_list(&args[1])?;
    Ok(parsed)
}

fn extract_create_comment_flags(
    args: &[String],
    initial: String,
    initial_set: bool,
) -> Result<(Vec<String>, String, bool), String> {
    let mut out = Vec::with_capacity(args.len());
    let mut comment = initial;
    let mut comment_set = initial_set;
    let mut index = 0;
    while index < args.len() {
        let arg = &args[index];
        if arg == "-c" {
            if comment_set {
                return Err("create comment specified more than once".to_string());
            }
            index += 1;
            let Some(value) = args.get(index) else {
                return Err("-c requires a comment".to_string());
            };
            comment = value.clone();
            comment_set = true;
        } else if let Some(value) = arg.strip_prefix("-c=") {
            if comment_set {
                return Err("create comment specified more than once".to_string());
            }
            comment = value.to_string();
            comment_set = true;
        } else {
            out.push(arg.clone());
        }
        index += 1;
    }
    Ok((out, comment, comment_set))
}

fn parse_create_ip_arg(value: &str) -> Option<String> {
    if let Ok(ip) = value.parse::<Ipv4Addr>() {
        return Some(ip.to_string());
    }
    let cidr = parse_ipv4_cidr(value).ok()?;
    (cidr.prefix == 32).then(|| cidr.ip.to_string())
}

fn parse_create_access_list(value: &str) -> Result<Vec<String>, String> {
    if value.is_empty() {
        return Err("access list must not be empty".to_string());
    }
    let mut seen = HashSet::new();
    let mut access = Vec::new();
    for part in value.split(',') {
        if part.is_empty() {
            return Err("empty access entry".to_string());
        }
        if part != "*" && !valid_resource_name(part) {
            return Err(format!("invalid resource name {part:?}"));
        }
        if !seen.insert(part.to_string()) {
            return Err(format!("duplicate access entry {part:?}"));
        }
        access.push(part.to_string());
    }
    access.sort();
    Ok(access)
}

fn valid_user_name(value: &str) -> bool {
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

fn ipv4_cidr_contains(cidr: &crate::utils::IPv4Cidr, ip: Ipv4Addr) -> bool {
    let mask = if cidr.prefix == 0 {
        0
    } else {
        u32::MAX << (32 - u32::from(cidr.prefix))
    };
    (ipv4_to_u32(ip) & mask) == ipv4_to_u32(cidr.network)
}
