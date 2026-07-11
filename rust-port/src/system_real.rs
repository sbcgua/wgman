use std::io::IsTerminal;

use crate::system::SystemAdapter;

pub struct RealSystemAdapter;

impl SystemAdapter for RealSystemAdapter {
    fn is_root(&self) -> bool {
        #[cfg(unix)]
        {
            std::process::Command::new("id")
                .arg("-u")
                .output()
                .ok()
                .and_then(|output| String::from_utf8(output.stdout).ok())
                .and_then(|uid| uid.trim().parse::<u32>().ok())
                == Some(0)
        }

        #[cfg(not(unix))]
        {
            false
        }
    }

    fn stdout_is_terminal(&self) -> bool {
        std::io::stdout().is_terminal()
    }

    fn stderr_is_terminal(&self) -> bool {
        std::io::stderr().is_terminal()
    }
}
