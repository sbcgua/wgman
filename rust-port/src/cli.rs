use std::ffi::OsString;
use std::io::{self, BufRead, Write};
use std::process::ExitCode;
use std::time::SystemTime;

use crate::commands;
use crate::system::SystemAdapter;
use crate::system::DEFAULT_CONFIG_DIR;
use crate::system_real::RealSystemAdapter;

pub const HELP_TEXT: &str = "\
wgman-rs - wireguard and resource access manager

Usage: wgman-rs <command> [flags] [args...]

Commands:
  check        Validate config/db and live WireGuard/ipset state
  list [user]  List users, resources, and optional access for a user
  show         Show live WireGuard peers mapped to user names
  init-ipsets  Create the managed ipsets defined in config.yaml
  deploy       Reconcile ipset state from db.yaml (supports --dry-run, --yes)
  create       Create a new VPN user: create <name> [-c comment] [ip] [res1,res2...]
  add          Alias for create
  remove       Remove an existing VPN user: remove <name> (supports --dry-run, --yes)
  mod          Modify access or active state: mod <name> <+res1,-res2...|activate|deactivate> (supports --dry-run)
  help         Show this help message

Global flags:
  --config-dir <dir>  Config directory (default /etc/wireguard/wgman)
  --yes               Skip interactive confirmation prompts
  --dry-run           Show planned changes without applying them (deploy, remove, mod only)
  --no-color          Disable colorized terminal output

Run 'wgman-rs help' or 'wgman-rs -h' for this message.
";

pub struct App<S> {
    system: S,
    now: fn() -> SystemTime,
}

impl<S> App<S>
where
    S: SystemAdapter,
{
    pub fn new(system: S) -> Self {
        Self {
            system,
            now: SystemTime::now,
        }
    }

    pub fn new_with_now(system: S, now: fn() -> SystemTime) -> Self {
        Self { system, now }
    }

    pub fn run<I, W, E>(&self, args: I, stdout: &mut W, stderr: &mut E) -> i32
    where
        I: IntoIterator<Item = OsString>,
        W: Write,
        E: Write,
    {
        let mut stdin = io::BufReader::new(io::empty());
        self.run_with_input(args, &mut stdin, stdout, stderr)
    }

    pub fn run_with_input<I, R, W, E>(
        &self,
        args: I,
        stdin: &mut R,
        stdout: &mut W,
        stderr: &mut E,
    ) -> i32
    where
        I: IntoIterator<Item = OsString>,
        R: BufRead,
        W: Write,
        E: Write,
    {
        let args: Vec<String> = args
            .into_iter()
            .skip(1)
            .map(|arg| arg.to_string_lossy().into_owned())
            .collect();

        let parsed = match parse_command_args(&args, stderr) {
            Ok(parsed) => parsed,
            Err(()) => return 2,
        };
        if parsed.help {
            return self.print_help(stdout);
        }

        let Some(command) = parsed.name.as_deref() else {
            return self.print_help(stdout);
        };

        match command {
            "check" => {
                commands::check::cmd_check(&parsed.flags, &parsed.args, self, stdout, stderr)
            }
            "list" => commands::list::cmd_list(&parsed.flags, &parsed.args, self, stdout, stderr),
            "show" => commands::show::cmd_show(&parsed.flags, &parsed.args, self, stdout, stderr),
            "init-ipsets" | "create" | "add"
                if reject_unsupported_dry_run(command, &parsed.flags, stderr) =>
            {
                2
            }
            "init-ipsets" => commands::init_ipsets::cmd_init_ipsets(
                &parsed.flags,
                &parsed.args,
                self,
                stdout,
                stderr,
            ),
            "deploy" => commands::deploy::cmd_deploy(
                &parsed.flags,
                &parsed.args,
                self,
                stdin,
                stdout,
                stderr,
            ),
            "create" | "add" => {
                commands::create::cmd_create(&parsed.flags, &parsed.args, self, stdout, stderr)
            }
            "remove" => commands::remove::cmd_remove(
                &parsed.flags,
                &parsed.args,
                self,
                stdin,
                stdout,
                stderr,
            ),
            "mod" => commands::modify::cmd_mod(&parsed.flags, &parsed.args, self, stdout, stderr),
            other => {
                let _ = writeln!(
                    stderr,
                    "wgman-rs: unknown command {other:?}\nRun 'wgman-rs help' for usage."
                );
                2
            }
        }
    }

    pub fn system(&self) -> &S {
        &self.system
    }

    pub fn into_system(self) -> S {
        self.system
    }

    pub fn stdout_color_enabled(&self, no_color: bool) -> bool {
        !no_color && self.system.stdout_is_terminal()
    }

    pub fn stderr_color_enabled(&self, no_color: bool) -> bool {
        !no_color && self.system.stderr_is_terminal()
    }

    pub fn now(&self) -> SystemTime {
        (self.now)()
    }

    fn print_help<W>(&self, stdout: &mut W) -> i32
    where
        W: Write,
    {
        let _ = self.system.stdout_is_terminal();
        let _ = stdout.write_all(HELP_TEXT.as_bytes());
        0
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct GlobalFlags {
    pub config_dir: String,
    pub yes: bool,
    pub dry_run: bool,
    pub no_color: bool,
    pub create_comment: String,
    pub create_comment_set: bool,
}

impl Default for GlobalFlags {
    fn default() -> Self {
        Self {
            config_dir: DEFAULT_CONFIG_DIR.to_string(),
            yes: false,
            dry_run: false,
            no_color: false,
            create_comment: String::new(),
            create_comment_set: false,
        }
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
struct ParsedCommand {
    name: Option<String>,
    args: Vec<String>,
    flags: GlobalFlags,
    help: bool,
}

fn parse_command_args<W: Write>(args: &[String], stderr: &mut W) -> Result<ParsedCommand, ()> {
    if args.is_empty() {
        return Ok(ParsedCommand {
            name: None,
            args: Vec::new(),
            flags: GlobalFlags::default(),
            help: true,
        });
    }

    let mut flags = GlobalFlags::default();
    let mut index = 0;
    if parse_flags(args, &mut index, &mut flags, stderr)? {
        return Ok(ParsedCommand {
            name: None,
            args: Vec::new(),
            flags,
            help: true,
        });
    }

    if index >= args.len() {
        return Ok(ParsedCommand {
            name: None,
            args: Vec::new(),
            flags,
            help: true,
        });
    }

    let command = args[index].clone();
    if matches!(command.as_str(), "help" | "-h" | "--help") {
        return Ok(ParsedCommand {
            name: None,
            args: Vec::new(),
            flags,
            help: true,
        });
    }
    index += 1;

    if parse_flags(args, &mut index, &mut flags, stderr)? {
        return Ok(ParsedCommand {
            name: None,
            args: Vec::new(),
            flags,
            help: true,
        });
    }
    let command_args = args[index..].to_vec();

    if flags.create_comment_set && command != "create" && command != "add" {
        let _ = writeln!(stderr, "error: -c is only supported by create/add");
        return Err(());
    }
    if flags.create_comment_set {
        flags.create_comment = flags.create_comment.trim().to_string();
        if flags.create_comment.is_empty() {
            let _ = writeln!(stderr, "error: create comment must not be empty");
            return Err(());
        }
    }

    Ok(ParsedCommand {
        name: Some(command),
        args: command_args,
        flags,
        help: false,
    })
}

fn parse_flags<W: Write>(
    args: &[String],
    index: &mut usize,
    flags: &mut GlobalFlags,
    stderr: &mut W,
) -> Result<bool, ()> {
    while *index < args.len() {
        let arg = &args[*index];
        match arg.as_str() {
            "-h" | "--help" => {
                *index = args.len();
                return Ok(true);
            }
            "--yes" | "-yes" => flags.yes = true,
            "--dry-run" | "-dry-run" => flags.dry_run = true,
            "--no-color" | "-no-color" => flags.no_color = true,
            "--config-dir" | "-config-dir" => {
                *index += 1;
                let Some(value) = args.get(*index) else {
                    let _ = writeln!(stderr, "error: --config-dir requires a value");
                    return Err(());
                };
                flags.config_dir = value.clone();
            }
            "-c" => {
                if flags.create_comment_set {
                    let _ = writeln!(stderr, "error: -c specified more than once");
                    return Err(());
                }
                *index += 1;
                let Some(value) = args.get(*index) else {
                    let _ = writeln!(stderr, "error: -c requires a value");
                    return Err(());
                };
                flags.create_comment = value.clone();
                flags.create_comment_set = true;
            }
            _ if arg.starts_with("--config-dir=") => {
                flags.config_dir = arg["--config-dir=".len()..].to_string();
            }
            _ if arg.starts_with("-config-dir=") => {
                flags.config_dir = arg["-config-dir=".len()..].to_string();
            }
            _ if arg.starts_with("--yes=") => {
                flags.yes = parse_bool_flag_value("--yes", &arg["--yes=".len()..], stderr)?;
            }
            _ if arg.starts_with("-yes=") => {
                flags.yes = parse_bool_flag_value("-yes", &arg["-yes=".len()..], stderr)?;
            }
            _ if arg.starts_with("--dry-run=") => {
                flags.dry_run =
                    parse_bool_flag_value("--dry-run", &arg["--dry-run=".len()..], stderr)?;
            }
            _ if arg.starts_with("-dry-run=") => {
                flags.dry_run =
                    parse_bool_flag_value("-dry-run", &arg["-dry-run=".len()..], stderr)?;
            }
            _ if arg.starts_with("--no-color=") => {
                flags.no_color =
                    parse_bool_flag_value("--no-color", &arg["--no-color=".len()..], stderr)?;
            }
            _ if arg.starts_with("-no-color=") => {
                flags.no_color =
                    parse_bool_flag_value("-no-color", &arg["-no-color=".len()..], stderr)?;
            }
            _ if arg.starts_with("-c=") => {
                if flags.create_comment_set {
                    let _ = writeln!(stderr, "error: -c specified more than once");
                    return Err(());
                }
                flags.create_comment = arg["-c=".len()..].to_string();
                flags.create_comment_set = true;
            }
            _ if arg.starts_with('-') => {
                let _ = writeln!(stderr, "error: unknown flag {arg}");
                return Err(());
            }
            _ => break,
        }
        *index += 1;
    }
    Ok(false)
}

fn parse_bool_flag_value<W: Write>(flag: &str, value: &str, stderr: &mut W) -> Result<bool, ()> {
    match value {
        "1" => Ok(true),
        "0" => Ok(false),
        _ if value.eq_ignore_ascii_case("t") || value.eq_ignore_ascii_case("true") => Ok(true),
        _ if value.eq_ignore_ascii_case("f") || value.eq_ignore_ascii_case("false") => Ok(false),
        _ => {
            let _ = writeln!(stderr, "error: invalid boolean value {value:?} for {flag}");
            Err(())
        }
    }
}

pub fn reject_unsupported_dry_run<W: Write>(
    command: &str,
    flags: &GlobalFlags,
    stderr: &mut W,
) -> bool {
    if !flags.dry_run {
        return false;
    }
    let _ = writeln!(stderr, "error: {command} does not support --dry-run");
    true
}

pub fn main_entry<I>(args: I) -> ExitCode
where
    I: IntoIterator<Item = OsString>,
{
    let app = App::new(RealSystemAdapter);
    let mut stdin = io::stdin().lock();
    let code = app.run_with_input(
        args,
        &mut stdin,
        &mut io::stdout().lock(),
        &mut io::stderr().lock(),
    );
    ExitCode::from(code as u8)
}
