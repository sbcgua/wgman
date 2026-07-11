use std::ffi::OsString;
use std::io::{self, Write};
use std::process::ExitCode;

use crate::system::SystemAdapter;
use crate::system_real::RealSystemAdapter;

pub const HELP_TEXT: &str = "\
wgman-rs - WireGuard and resource access manager

Usage:
  wgman-rs help
  wgman-rs <command> [params...]

Commands:
  help        Show this help message
  check       Validate desired and live state
  list        List users, VMs, resources, and access
  show        Show WireGuard peers with user names
  init-ipsets Initialize managed ipsets
  deploy      Reconcile live state to db.yaml
  create      Create a user
  add         Alias for create
  remove      Remove a user
  mod         Modify user access or activation

Global flags:
  --yes        Skip confirmation prompts
  --dry-run    Show planned changes without applying them
  --no-color   Suppress colorized output
  -h, --help   Show this help message
";

pub struct App<S> {
    system: S,
}

impl<S> App<S>
where
    S: SystemAdapter,
{
    pub fn new(system: S) -> Self {
        Self { system }
    }

    pub fn run<I, W, E>(&self, args: I, stdout: &mut W, stderr: &mut E) -> i32
    where
        I: IntoIterator<Item = OsString>,
        W: Write,
        E: Write,
    {
        let mut args = args.into_iter();
        let _program = args.next();
        let Some(command) = args.next() else {
            return self.print_help(stdout);
        };

        match command.to_string_lossy().as_ref() {
            "help" | "-h" | "--help" => self.print_help(stdout),
            other => {
                let _ = writeln!(stderr, "unsupported command: {other}");
                2
            }
        }
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

pub fn main_entry<I>(args: I) -> ExitCode
where
    I: IntoIterator<Item = OsString>,
{
    let app = App::new(RealSystemAdapter);
    let code = app.run(args, &mut io::stdout().lock(), &mut io::stderr().lock());
    ExitCode::from(code as u8)
}
