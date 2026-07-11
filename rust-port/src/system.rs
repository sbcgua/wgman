pub const DEFAULT_CONFIG_DIR: &str = "/etc/wireguard/wgman";

pub trait SystemAdapter {
    fn is_root(&self) -> bool;
    fn stdout_is_terminal(&self) -> bool;
    fn stderr_is_terminal(&self) -> bool;
}
