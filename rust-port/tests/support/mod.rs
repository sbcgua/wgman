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
    pub ipset_create_error: Option<String>,
    pub ipset_add_error: Option<String>,
    pub ipset_add_errors: std::collections::HashMap<String, String>,
    pub ipset_del_error: Option<String>,
    pub ipset_del_errors: std::collections::HashMap<String, String>,
    pub wg_set_error: Option<String>,
    pub wg_del_error: Option<String>,
    pub gen_key_result: String,
    pub gen_key_error: Option<String>,
    pub pub_key_result: String,
    pub pub_key_error: Option<String>,
    pub applied_ops: std::cell::RefCell<Vec<String>>,
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

    fn ipset_create(
        &self,
        set_name: &str,
        set_type: &str,
        with_comment: bool,
    ) -> Result<(), String> {
        if let Some(err) = &self.ipset_create_error {
            return Err(err.clone());
        }
        let comment = if with_comment { "comment" } else { "nocomment" };
        self.applied_ops
            .borrow_mut()
            .push(format!("create:{set_name}:{set_type}:{comment}"));
        Ok(())
    }

    fn ipset_add(&self, set_name: &str, entry: &str, comment: &str) -> Result<(), String> {
        if let Some(err) = &self.ipset_add_error {
            return Err(err.clone());
        }
        if let Some(err) = self.ipset_add_errors.get(&format!("{set_name}:{entry}")) {
            return Err(err.clone());
        }
        self.applied_ops
            .borrow_mut()
            .push(format!("add:{set_name}:{entry}:{comment}"));
        Ok(())
    }

    fn ipset_del(&self, set_name: &str, entry: &str) -> Result<(), String> {
        if let Some(err) = &self.ipset_del_error {
            return Err(err.clone());
        }
        if let Some(err) = self.ipset_del_errors.get(&format!("{set_name}:{entry}")) {
            return Err(err.clone());
        }
        self.applied_ops
            .borrow_mut()
            .push(format!("del:{set_name}:{entry}"));
        Ok(())
    }

    fn wg_set_peer(&self, iface: &str, pub_key: &str, allowed_ip: &str) -> Result<(), String> {
        if let Some(err) = &self.wg_set_error {
            return Err(err.clone());
        }
        self.applied_ops
            .borrow_mut()
            .push(format!("wgset:{iface}:{pub_key}:{allowed_ip}"));
        Ok(())
    }

    fn wg_del_peer(&self, iface: &str, pub_key: &str) -> Result<(), String> {
        if let Some(err) = &self.wg_del_error {
            return Err(err.clone());
        }
        self.applied_ops
            .borrow_mut()
            .push(format!("wgdel:{iface}:{pub_key}"));
        Ok(())
    }

    fn wg_gen_key(&self) -> Result<String, String> {
        match &self.gen_key_error {
            Some(err) => Err(err.clone()),
            None if !self.gen_key_result.is_empty() => Ok(self.gen_key_result.clone()),
            None => Ok("FAKE_PRIVATE_KEY".to_string()),
        }
    }

    fn wg_pub_key(&self, _private_key: &str) -> Result<String, String> {
        match &self.pub_key_error {
            Some(err) => Err(err.clone()),
            None if !self.pub_key_result.is_empty() => Ok(self.pub_key_result.clone()),
            None => Ok("FAKE_PUBLIC_KEY".to_string()),
        }
    }
}
