pub const DEFAULT_CONFIG_DIR: &str = "/etc/wireguard/wgman";

pub trait SystemAdapter {
    fn is_root(&self) -> bool;
    fn stdout_is_terminal(&self) -> bool;
    fn stderr_is_terminal(&self) -> bool;
    fn interface_subnet(&self, iface: &str) -> Result<String, String>;
    fn wg_dump(&self, iface: &str) -> Result<String, String>;
    fn ipset_list(&self, set_name: &str) -> Result<String, String>;
    fn ipset_create(
        &self,
        set_name: &str,
        set_type: &str,
        with_comment: bool,
    ) -> Result<(), String>;
    fn ipset_add(&self, set_name: &str, entry: &str, comment: &str) -> Result<(), String>;
    fn ipset_del(&self, set_name: &str, entry: &str) -> Result<(), String>;
    fn wg_set_peer(&self, iface: &str, pub_key: &str, allowed_ip: &str) -> Result<(), String>;
    fn wg_del_peer(&self, iface: &str, pub_key: &str) -> Result<(), String>;
    fn wg_gen_key(&self) -> Result<String, String>;
    fn wg_pub_key(&self, private_key: &str) -> Result<String, String>;
}
