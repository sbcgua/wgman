use std::io::{BufRead, Write};

use crate::check::{check, compute_expected_ipsets};
use crate::cli::{App, GlobalFlags};
use crate::config::load_config;
use crate::db::{clone_db, normalize_db_access, save_db_atomic, validate_db};
use crate::deploy::{apply_state_deltas_tracked, diff_expected_ipsets, rollback_state_deltas};
use crate::model::{Config, IPSetDelta, WGPeerDelta, WGPeerDeltaAction, DB};
use crate::output::{print_apply_and_rollback_error, print_check_errors, print_ipset_deltas};
use crate::system::SystemAdapter;
use crate::utils::confirm_action;

#[derive(Clone, Debug, PartialEq, Eq)]
struct RemovePlan {
    updated_db: DB,
    ipset_deltas: Vec<IPSetDelta>,
    peer_deltas: Vec<WGPeerDelta>,
    user: String,
    ip: String,
    access: Vec<String>,
}

pub fn cmd_remove<S, R, W, E>(
    flags: &GlobalFlags,
    args: &[String],
    app: &App<S>,
    stdin: &mut R,
    stdout: &mut W,
    stderr: &mut E,
) -> i32
where
    S: SystemAdapter,
    R: BufRead,
    W: Write,
    E: Write,
{
    if args.len() != 1 {
        let _ = writeln!(stderr, "error: remove requires <name>");
        return 2;
    }
    if !app.system().is_root() {
        let _ = writeln!(stderr, "error: wgman must be run as root");
        return 1;
    }

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
            "remove: FAILED (resolve hard errors or drift before removing users)"
        );
        return 1;
    }

    let plan = match plan_remove_user(&cfg, &db, &args[0]) {
        Ok(plan) => plan,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };

    print_remove_plan(&plan, stdout);
    if flags.dry_run {
        let _ = writeln!(stdout, "remove: dry-run, no changes applied");
        return 0;
    }
    if !flags.yes && !confirm_action(stdin, stdout, "Remove this user? [y/N] ") {
        let _ = writeln!(stdout, "remove: aborted");
        return 0;
    }

    if let Err((err, rollback_err)) =
        apply_remove_user_plan(&flags.config_dir, &cfg.interface, &plan, app.system())
    {
        print_apply_and_rollback_error(&err, rollback_err.as_deref(), stderr);
        return 1;
    }

    let _ = writeln!(stdout, "remove: removed {}", plan.user);
    0
}

fn apply_remove_user_plan<S: SystemAdapter>(
    config_dir: &str,
    iface: &str,
    plan: &RemovePlan,
    system: &S,
) -> Result<(), (String, Option<String>)> {
    let applied =
        match apply_state_deltas_tracked(iface, &plan.ipset_deltas, &plan.peer_deltas, system) {
            Ok(applied) => applied,
            Err(err) => {
                let rollback_err = rollback_state_deltas(iface, &err.applied, system).err();
                return Err((err.error, rollback_err));
            }
        };
    if let Err(err) = save_db_atomic(config_dir, &plan.updated_db) {
        let rollback_err = rollback_state_deltas(iface, &applied, system).err();
        return Err((err, rollback_err));
    }
    Ok(())
}

fn plan_remove_user(cfg: &Config, db: &DB, user: &str) -> Result<RemovePlan, String> {
    if !valid_user_name(user) {
        return Err(format!("invalid user name {user:?}"));
    }
    let entry = db
        .users
        .get(user)
        .ok_or_else(|| format!("user {user:?} not found"))?;

    let mut updated = clone_db(db);
    updated.users.remove(user);
    updated.access.remove(user);
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
    let mut access = db.access.get(user).cloned().unwrap_or_default();
    access.sort();
    Ok(RemovePlan {
        updated_db: updated,
        ipset_deltas: diff_expected_ipsets(cfg, &old_expected, &new_expected),
        peer_deltas: vec![WGPeerDelta {
            user: user.to_string(),
            pub_key: entry.pub_key.clone(),
            allowed_ip: entry.ip.clone(),
            action: WGPeerDeltaAction::Remove,
        }],
        user: user.to_string(),
        ip: entry.ip.clone(),
        access,
    })
}

fn print_remove_plan<W: Write>(plan: &RemovePlan, stdout: &mut W) {
    let _ = writeln!(
        stdout,
        "remove: planned removal of {} ({})",
        plan.user, plan.ip
    );
    if plan.access.is_empty() {
        let _ = writeln!(stdout, "remove: access: (none)");
    } else {
        let _ = writeln!(stdout, "remove: access: {}", plan.access.join(", "));
    }
    if !plan.ipset_deltas.is_empty() {
        let _ = writeln!(
            stdout,
            "remove: planned access changes ({}):",
            plan.ipset_deltas.len()
        );
        print_ipset_deltas(&plan.ipset_deltas, stdout);
    }
}

fn valid_user_name(value: &str) -> bool {
    !value.is_empty()
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
}
