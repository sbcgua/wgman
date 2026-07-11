use wgman_rs::system::SystemAdapter;

#[derive(Default)]
pub struct FakeSystem {
    pub root: bool,
    pub stdout_tty: bool,
    pub stderr_tty: bool,
    pub subnet_result: String,
    pub subnet_error: Option<String>,
    pub wg_dump_result: String,
    pub wg_dump_error: Option<String>,
    pub ipset_results: std::collections::HashMap<String, String>,
    pub ipset_errors: std::collections::HashMap<String, String>,
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

    fn interface_subnet(&self, _iface: &str) -> Result<String, String> {
        match &self.subnet_error {
            Some(err) => Err(err.clone()),
            None => Ok(self.subnet_result.clone()),
        }
    }

    fn wg_dump(&self, _iface: &str) -> Result<String, String> {
        match &self.wg_dump_error {
            Some(err) => Err(err.clone()),
            None => Ok(self.wg_dump_result.clone()),
        }
    }

    fn ipset_list(&self, set_name: &str) -> Result<String, String> {
        match self.ipset_errors.get(set_name) {
            Some(err) => Err(err.clone()),
            None => Ok(self
                .ipset_results
                .get(set_name)
                .cloned()
                .unwrap_or_default()),
        }
    }
}
