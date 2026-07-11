use wgman_rs::system::SystemAdapter;

#[derive(Default)]
pub struct FakeSystem {
    pub root: bool,
    pub stdout_tty: bool,
    pub stderr_tty: bool,
}

impl SystemAdapter for FakeSystem {
    fn is_root(&self) -> bool {
        self.root
    }

    fn stdout_is_terminal(&self) -> bool {
        self.stdout_tty
    }

    fn stderr_is_terminal(&self) -> bool {
        self.stderr_tty
    }
}
