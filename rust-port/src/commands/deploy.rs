use std::io::{BufRead, Write};

use crate::check::check;
use crate::cli::{App, GlobalFlags};
use crate::config::load_config;
use crate::db::load_db;
use crate::deploy::apply_state_deltas;
use crate::output::{print_ipset_deltas, print_peer_deltas};
use crate::system::SystemAdapter;
use crate::utils::confirm_action;

pub fn cmd_deploy<S, R, W, E>(
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
    if !args.is_empty() {
        let _ = writeln!(stderr, "error: deploy takes no positional arguments");
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
    if !result.clean() {
        let _ = writeln!(stderr, "deploy: hard errors:");
        for error in &result.hard_errors {
            let _ = writeln!(stderr, "  - {error}");
        }
        let _ = writeln!(
            stderr,
            "deploy: FAILED (resolve hard errors before deploying)"
        );
        return 1;
    }

    let change_count = result.ipset_deltas.len() + result.peer_deltas.len();
    if change_count == 0 {
        let _ = writeln!(stdout, "deploy: no changes needed");
        return 0;
    }

    let _ = writeln!(stdout, "deploy: planned changes ({change_count}):");
    print_ipset_deltas(&result.ipset_deltas, stdout);
    print_peer_deltas(&result.peer_deltas, stdout);

    if flags.dry_run {
        let _ = writeln!(stdout, "deploy: dry-run, no changes applied");
        return 0;
    }

    if !flags.yes && !confirm_action(stdin, stdout, "Apply these changes? [y/N] ") {
        let _ = writeln!(stdout, "deploy: aborted");
        return 0;
    }

    if let Err(err) = apply_state_deltas(
        &cfg.interface,
        &result.ipset_deltas,
        &result.peer_deltas,
        app.system(),
    ) {
        let _ = writeln!(stderr, "error: {err}");
        return 1;
    }

    let _ = writeln!(stdout, "deploy: applied {change_count} change(s)");
    0
}
