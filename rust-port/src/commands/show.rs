use std::collections::HashMap;
use std::io::Write;
use std::time::SystemTime;

use crate::check::check;
use crate::cli::{reject_unsupported_dry_run, App, GlobalFlags};
use crate::config::load_config;
use crate::db::load_db;
use crate::format::{format_bytes, format_bytes_color, format_handshake, format_handshake_color};
use crate::format::{write_table, TableCell};
use crate::model::{CheckResult, DB};
use crate::output::{print_check_errors, user_name_cell};
use crate::parse_wg::WGPeer;
use crate::system::SystemAdapter;
use crate::utils::{endpoint_host, sorted_keys};

pub fn cmd_show<S, W, E>(
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
        let _ = writeln!(stderr, "error: show takes no positional arguments");
        return 2;
    }
    if reject_unsupported_dry_run("show", flags, stderr) {
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
    run_show_with_color(
        &db,
        &result,
        app.now(),
        stdout,
        stderr,
        app.stdout_color_enabled(flags.no_color),
    )
}

pub fn run_show<W, E>(
    db: &DB,
    result: &CheckResult,
    now: SystemTime,
    stdout: &mut W,
    stderr: &mut E,
) -> i32
where
    W: Write,
    E: Write,
{
    run_show_with_color(db, result, now, stdout, stderr, false)
}

pub fn run_show_with_color<W, E>(
    db: &DB,
    result: &CheckResult,
    now: SystemTime,
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
        let _ = writeln!(stderr, "show: FAILED");
        return 1;
    }

    let Some(dump) = &result.wg_dump else {
        let _ = writeln!(stderr, "show: no WireGuard data available");
        return 1;
    };

    let pub_to_user: HashMap<&str, &str> = db
        .users
        .iter()
        .map(|(name, user)| (user.pub_key.as_str(), name.as_str()))
        .collect();
    let mut name_to_peer: HashMap<&str, &WGPeer> = HashMap::new();
    for peer in &dump.peers {
        if let Some(name) = pub_to_user.get(peer.public_key.as_str()) {
            name_to_peer.insert(*name, peer);
        }
    }

    let mut rows = Vec::new();
    for name in sorted_keys(&db.users) {
        let user = &db.users[&name];
        let name_cell = user_name_cell(&name, user.inactive, color);
        let Some(peer) = name_to_peer.get(name.as_str()) else {
            rows.push(vec![
                name_cell,
                TableCell::plain(user.ip.clone()),
                TableCell::plain("-"),
                TableCell::plain("-"),
                TableCell::plain("-"),
                TableCell::plain("-"),
            ]);
            continue;
        };

        let rx_plain = format_bytes(peer.rx_bytes);
        let tx_plain = format_bytes(peer.tx_bytes);
        let handshake_plain = format_handshake(peer.latest_handshake, now);
        let endpoint = endpoint_host(&peer.endpoint);
        rows.push(vec![
            name_cell,
            TableCell::plain(user.ip.clone()),
            TableCell::plain(endpoint),
            TableCell::new(rx_plain, format_bytes_color(peer.rx_bytes, color)),
            TableCell::new(tx_plain, format_bytes_color(peer.tx_bytes, color)),
            TableCell::new(
                handshake_plain,
                format_handshake_color(peer.latest_handshake, now, color),
            ),
        ]);
    }

    let _ = write_table(
        stdout,
        &["NAME", "IP", "ENDPOINT", "RX", "TX", "LAST HANDSHAKE"],
        &rows,
    );
    0
}
