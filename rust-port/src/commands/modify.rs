use std::collections::HashSet;
use std::io::Write;

use crate::check::{check, compute_expected_ipsets};
use crate::cli::{App, GlobalFlags};
use crate::config::load_config;
use crate::db::{clone_db, normalize_access_set, normalize_db_access, save_db_atomic, validate_db};
use crate::deploy::{
    apply_ipset_deltas, apply_state_deltas_tracked, diff_expected_ipsets, rollback_state_deltas,
};
use crate::model::{Config, IPSetDelta, WGPeerDelta, WGPeerDeltaAction, DB};
use crate::output::{
    print_apply_and_rollback_error, print_check_errors, print_ipset_deltas, print_peer_deltas,
};
use crate::system::SystemAdapter;

#[derive(Clone, Debug, PartialEq, Eq)]
struct ModAccessOp {
    add: bool,
    resource: String,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct ModAccessPlan {
    updated_db: DB,
    ipset_deltas: Vec<IPSetDelta>,
    user: String,
    access_changed: bool,
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct ModTogglePlan {
    updated_db: DB,
    ipset_deltas: Vec<IPSetDelta>,
    peer_deltas: Vec<WGPeerDelta>,
    no_op: bool,
}

struct ModToggleContext<'a, S> {
    flags: &'a GlobalFlags,
    cfg: &'a Config,
    db: &'a DB,
    system: &'a S,
}

pub fn cmd_mod<S, W, E>(
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
    if args.len() != 2 {
        let _ = writeln!(
            stderr,
            "error: mod requires <name> <+res1,-res2...|activate|deactivate>"
        );
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
            "mod: FAILED (resolve hard errors or drift before modifying access)"
        );
        return 1;
    }

    if is_mod_toggle_action(&args[1]) {
        let context = ModToggleContext {
            flags,
            cfg: &cfg,
            db: &db,
            system: app.system(),
        };
        return cmd_mod_toggle(context, &args[0], &args[1], stdout, stderr);
    }

    let ops = match parse_mod_expression(&args[1]) {
        Ok(ops) => ops,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 2;
        }
    };
    let plan = match plan_mod_access(&cfg, &db, &args[0], &ops) {
        Ok(plan) => plan,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };

    if plan.ipset_deltas.is_empty() && !plan.access_changed {
        let _ = writeln!(stdout, "mod: no changes needed");
        return 0;
    }

    let change_count = if plan.access_changed && plan.ipset_deltas.is_empty() {
        1
    } else {
        plan.ipset_deltas.len()
    };
    let _ = writeln!(stdout, "mod: planned changes ({change_count}):");
    if plan.ipset_deltas.is_empty() {
        let _ = writeln!(stdout, "  update db access for {}", plan.user);
    } else {
        print_ipset_deltas(&plan.ipset_deltas, stdout);
    }

    if flags.dry_run {
        let _ = writeln!(stdout, "mod: dry-run, no changes applied");
        return 0;
    }

    if let Err(err) = apply_mod_access_plan(&flags.config_dir, &plan, app.system()) {
        let _ = writeln!(stderr, "error: {err}");
        return 1;
    }

    let _ = writeln!(stdout, "mod: applied {change_count} change(s)");
    0
}

fn cmd_mod_toggle<S, W, E>(
    context: ModToggleContext<'_, S>,
    user: &str,
    action: &str,
    stdout: &mut W,
    stderr: &mut E,
) -> i32
where
    S: SystemAdapter,
    W: Write,
    E: Write,
{
    let plan = match plan_mod_toggle(context.cfg, context.db, user, action) {
        Ok(plan) => plan,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };
    if plan.no_op {
        let _ = writeln!(
            stdout,
            "mod: {user} already {}; no changes needed",
            toggle_state_name(action)
        );
        return 0;
    }

    let change_count = plan.ipset_deltas.len() + plan.peer_deltas.len();
    let _ = writeln!(stdout, "mod: planned changes ({change_count}):");
    print_ipset_deltas(&plan.ipset_deltas, stdout);
    print_peer_deltas(&plan.peer_deltas, stdout);

    if context.flags.dry_run {
        let _ = writeln!(stdout, "mod: dry-run, no changes applied");
        return 0;
    }

    if let Err((err, rollback_err)) = apply_mod_toggle_plan(
        &context.flags.config_dir,
        &context.cfg.interface,
        &plan,
        context.system,
    ) {
        print_apply_and_rollback_error(&err, rollback_err.as_deref(), stderr);
        return 1;
    }

    let _ = writeln!(stdout, "mod: applied {change_count} change(s)");
    0
}

fn apply_mod_access_plan<S: SystemAdapter>(
    config_dir: &str,
    plan: &ModAccessPlan,
    system: &S,
) -> Result<(), String> {
    save_db_atomic(config_dir, &plan.updated_db)?;
    apply_ipset_deltas(&plan.ipset_deltas, system)
}

fn apply_mod_toggle_plan<S: SystemAdapter>(
    config_dir: &str,
    iface: &str,
    plan: &ModTogglePlan,
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

fn parse_mod_expression(expr: &str) -> Result<Vec<ModAccessOp>, String> {
    if expr.is_empty() {
        return Err("mod expression is required".to_string());
    }
    let mut seen = HashSet::new();
    let mut ops = Vec::new();
    for part in expr.split(',') {
        if part.is_empty() {
            return Err("empty mod operation".to_string());
        }
        let Some(sign) = part.as_bytes().first() else {
            return Err("empty mod operation".to_string());
        };
        if part.len() < 2 || (*sign != b'+' && *sign != b'-') {
            return Err(format!("operation {part:?} must start with + or -"));
        }
        let resource = &part[1..];
        if resource != "*" && !valid_resource_name(resource) {
            return Err(format!("invalid resource name {resource:?}"));
        }
        if !seen.insert(resource.to_string()) {
            return Err(format!("duplicate resource {resource:?} in mod expression"));
        }
        ops.push(ModAccessOp {
            add: *sign == b'+',
            resource: resource.to_string(),
        });
    }
    Ok(ops)
}

fn is_mod_toggle_action(action: &str) -> bool {
    action == "activate" || action == "deactivate"
}

fn toggle_state_name(action: &str) -> &'static str {
    if action == "activate" {
        "active"
    } else {
        "inactive"
    }
}

fn plan_mod_toggle(
    cfg: &Config,
    db: &DB,
    user: &str,
    action: &str,
) -> Result<ModTogglePlan, String> {
    if !valid_user_name(user) {
        return Err(format!("invalid user name {user:?}"));
    }
    let entry = db
        .users
        .get(user)
        .ok_or_else(|| format!("user {user:?} not found"))?;

    match action {
        "activate" if !entry.inactive => {
            return Ok(ModTogglePlan {
                updated_db: db.clone(),
                ipset_deltas: Vec::new(),
                peer_deltas: Vec::new(),
                no_op: true,
            });
        }
        "deactivate" if entry.inactive => {
            return Ok(ModTogglePlan {
                updated_db: db.clone(),
                ipset_deltas: Vec::new(),
                peer_deltas: Vec::new(),
                no_op: true,
            });
        }
        "activate" | "deactivate" => {}
        _ => return Err(format!("unknown mod action {action:?}")),
    }

    let mut updated = clone_db(db);
    if let Some(updated_entry) = updated.users.get_mut(user) {
        updated_entry.inactive = action == "deactivate";
    }
    let errs = validate_db(&updated);
    if !errs.is_empty() {
        return Err(format!(
            "updated db.yaml would be invalid: {}",
            errs.join("; ")
        ));
    }

    let old_expected = compute_expected_ipsets(db);
    let new_expected = compute_expected_ipsets(&updated);
    let peer_action = if action == "activate" {
        WGPeerDeltaAction::Add
    } else {
        WGPeerDeltaAction::Remove
    };
    Ok(ModTogglePlan {
        updated_db: updated,
        ipset_deltas: diff_expected_ipsets(cfg, &old_expected, &new_expected),
        peer_deltas: vec![WGPeerDelta {
            user: user.to_string(),
            pub_key: entry.pub_key.clone(),
            allowed_ip: entry.ip.clone(),
            action: peer_action,
        }],
        no_op: false,
    })
}

fn plan_mod_access(
    cfg: &Config,
    db: &DB,
    user: &str,
    ops: &[ModAccessOp],
) -> Result<ModAccessPlan, String> {
    if !valid_user_name(user) {
        return Err(format!("invalid user name {user:?}"));
    }
    if !db.users.contains_key(user) {
        return Err(format!("user {user:?} not found"));
    }

    let mut updated = clone_db(db);
    let mut access_set: HashSet<String> = updated
        .access
        .get(user)
        .cloned()
        .unwrap_or_default()
        .into_iter()
        .collect();

    for op in ops {
        if op.resource != "*"
            && !updated.vms.contains_key(&op.resource)
            && !updated.resources.contains_key(&op.resource)
        {
            return Err(format!("unknown access target {:?}", op.resource));
        }
        if op.add {
            access_set.insert(op.resource.clone());
        } else {
            access_set.remove(&op.resource);
        }
    }

    updated
        .access
        .insert(user.to_string(), normalize_access_set(&access_set));
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
    let old_access = db.access.get(user).cloned().unwrap_or_default();
    let new_access = updated.access.get(user).cloned().unwrap_or_default();
    Ok(ModAccessPlan {
        updated_db: updated,
        ipset_deltas: diff_expected_ipsets(cfg, &old_expected, &new_expected),
        user: user.to_string(),
        access_changed: old_access != new_access,
    })
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
