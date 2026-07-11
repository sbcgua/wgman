use std::process::ExitCode;

fn main() -> ExitCode {
    wgman_rs::cli::main_entry(std::env::args_os())
}
