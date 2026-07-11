use std::ffi::OsString;

use wgman_rs::cli::{App, HELP_TEXT};

mod support;

use support::FakeSystem;

fn run(args: &[&str]) -> (i32, String, String) {
    let app = App::new(FakeSystem::default());
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();
    let code = app.run(args.iter().map(OsString::from), &mut stdout, &mut stderr);

    (
        code,
        String::from_utf8(stdout).expect("stdout is utf8"),
        String::from_utf8(stderr).expect("stderr is utf8"),
    )
}

#[test]
fn no_args_prints_help_and_succeeds() {
    let (code, stdout, stderr) = run(&["wgman-rs"]);

    assert_eq!(code, 0);
    assert_eq!(stdout, HELP_TEXT);
    assert_eq!(stderr, "");
}

#[test]
fn help_prints_help_and_succeeds() {
    let (code, stdout, stderr) = run(&["wgman-rs", "help"]);

    assert_eq!(code, 0);
    assert_eq!(stdout, HELP_TEXT);
    assert_eq!(stderr, "");
}

#[test]
fn help_flags_print_help_and_succeed() {
    for flag in ["-h", "--help"] {
        let (code, stdout, stderr) = run(&["wgman-rs", flag]);

        assert_eq!(code, 0, "{flag}");
        assert_eq!(stdout, HELP_TEXT, "{flag}");
        assert_eq!(stderr, "", "{flag}");
    }
}

#[test]
fn unsupported_command_is_usage_error() {
    let (code, stdout, stderr) = run(&["wgman-rs", "unknown"]);

    assert_eq!(code, 2);
    assert_eq!(stdout, "");
    assert_eq!(
        stderr,
        "wgman-rs: unknown command \"unknown\"\nRun 'wgman-rs help' for usage.\n"
    );
}
