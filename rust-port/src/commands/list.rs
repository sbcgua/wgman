use std::io::Write;

use crate::check::check;
use crate::cli::{reject_unsupported_dry_run, App, GlobalFlags};
use crate::config::load_config;
use crate::db::load_db;
use crate::model::{CheckResult, DB};
use crate::output::{
    color_access_item, format_access_summary, format_resource_ports, format_section_header,
    print_check_errors, user_name_cell,
};
use crate::system::SystemAdapter;
use crate::utils::sorted_keys;

pub fn cmd_list<S, W, E>(
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
    if args.len() > 1 {
        let _ = writeln!(stderr, "error: list takes at most one positional argument");
        return 2;
    }
    if reject_unsupported_dry_run("list", flags, stderr) {
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
    let db = match load_db(&flags.config_dir) {
        Ok(db) => db,
        Err(err) => {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
    };

    let result = check(&cfg, &db, app.system());
    let filter = args.first().map(String::as_str).unwrap_or("");
    run_list_with_color(
        &db,
        &result,
        filter,
        stdout,
        stderr,
        app.stdout_color_enabled(flags.no_color),
    )
}

pub fn run_list<W, E>(
    db: &DB,
    result: &CheckResult,
    filter: &str,
    stdout: &mut W,
    stderr: &mut E,
) -> i32
where
    W: Write,
    E: Write,
{
    run_list_with_color(db, result, filter, stdout, stderr, false)
}

pub fn run_list_with_color<W, E>(
    db: &DB,
    result: &CheckResult,
    filter: &str,
    stdout: &mut W,
    stderr: &mut E,
    color: bool,
) -> i32
where
    W: Write,
    E: Write,
{
    if !result.ok() {
        print_check_errors(result, stderr);
        let _ = writeln!(stderr, "list: FAILED");
        return 1;
    }

    if !filter.is_empty() {
        return run_list_user(db, filter, stdout, stderr, color);
    }

    let _ = writeln!(stdout, "Users:");
    for name in sorted_keys(&db.users) {
        let user = &db.users[&name];
        let name_cell = user_name_cell(&name, user.inactive, color);
        let padding = " ".repeat(20usize.saturating_sub(name_cell.plain.len()));
        let access = format_access_summary(db, db.access.get(&name), color);
        let _ = writeln!(
            stdout,
            "  {}{} {:<15} {}",
            name_cell.display, padding, user.ip, access
        );
    }

    let _ = writeln!(stdout);
    let _ = writeln!(stdout, "{}", format_section_header("VMs", color));
    for name in sorted_keys(&db.vms) {
        let _ = writeln!(stdout, "  {name:<20} {}", db.vms[&name]);
    }

    let _ = writeln!(stdout);
    let _ = writeln!(stdout, "{}", format_section_header("Resources", color));
    if db.resources.is_empty() {
        let _ = writeln!(stdout, "  (none)");
        return 0;
    }

    let names = sorted_keys(&db.resources);
    let name_width = names.iter().map(String::len).max().unwrap_or(0).max(20);
    let vm_width = db
        .resources
        .values()
        .map(|resource| resource.vm.len())
        .max()
        .unwrap_or(0);
    for name in names {
        let resource = &db.resources[&name];
        let _ = writeln!(
            stdout,
            "  {name:<name_width$} {vm:<vm_width$} {ports}",
            vm = resource.vm,
            ports = format_resource_ports(&resource.ports)
        );
    }

    0
}

fn run_list_user<W, E>(db: &DB, filter: &str, stdout: &mut W, stderr: &mut E, color: bool) -> i32
where
    W: Write,
    E: Write,
{
    let Some(user) = db.users.get(filter) else {
        let _ = writeln!(stderr, "error: user {filter:?} not found");
        let _ = writeln!(stderr, "list: FAILED");
        return 1;
    };

    let name_cell = user_name_cell(filter, user.inactive, color);
    let _ = writeln!(stdout, "{}:", name_cell.display);
    let Some(targets) = db.access.get(filter) else {
        let _ = writeln!(stdout, "  ({})", color_access_item(db, "none", color));
        return 0;
    };
    if targets.is_empty() {
        let _ = writeln!(stdout, "  ({})", color_access_item(db, "none", color));
        return 0;
    }

    let mut sorted = targets.clone();
    sorted.sort();
    for target in sorted {
        let _ = writeln!(stdout, "  {}", color_access_item(db, &target, color));
    }
    0
}
