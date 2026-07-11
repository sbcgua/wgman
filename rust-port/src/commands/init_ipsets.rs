use std::io::Write;

use crate::cli::{App, GlobalFlags};
use crate::config::load_config;
use crate::system::SystemAdapter;

pub fn cmd_init_ipsets<S, W, E>(
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
        let _ = writeln!(stderr, "error: init-ipsets takes no positional arguments");
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

    let sets = [
        (&cfg.sets.all, "hash:ip", false),
        (&cfg.sets.ip_matrix, "hash:net,net", true),
        (&cfg.sets.port_matrix, "hash:ip,port,ip", true),
    ];
    for (set_name, set_type, with_comment) in sets {
        if let Err(err) = app.system().ipset_create(set_name, set_type, with_comment) {
            let _ = writeln!(stderr, "error: {err}");
            return 1;
        }
        let _ = writeln!(stdout, "ipset {set_name:?}: created ({set_type})");
    }

    let _ = writeln!(stdout, "init-ipsets: OK");
    0
}
