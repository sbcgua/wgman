use std::io::Write;

use crate::check::check;
use crate::cli::{reject_unsupported_dry_run, App, GlobalFlags};
use crate::config::load_config;
use crate::db::load_db;
use crate::output::{format_status, print_check_errors};
use crate::system::SystemAdapter;

pub fn cmd_check<S, W, E>(
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
    if !args.is_empty() {
        let _ = writeln!(stderr, "error: check takes no positional arguments");
        return 2;
    }
    if reject_unsupported_dry_run("check", flags, stderr) {
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
    print_check_errors(&result, stderr);

    if !result.ok() {
        let status = format_status("FAILED", app.stderr_color_enabled(flags.no_color));
        let _ = writeln!(stderr, "check: {status}");
        return 1;
    }

    let status = format_status("OK", app.stdout_color_enabled(flags.no_color));
    let _ = writeln!(stdout, "check: {status}");
    0
}
