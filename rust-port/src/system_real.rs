use std::io::{IsTerminal, Write};

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

    fn interface_subnet(&self, iface: &str) -> Result<String, String> {
        let output = std::process::Command::new("ip")
            .args(["-o", "-4", "addr", "show", "dev", iface])
            .output()
            .map_err(|err| format!("run ip addr show: {err}"))?;
        if !output.status.success() {
            return Err(command_stderr("ip addr show", &output));
        }

        let stdout = String::from_utf8(output.stdout)
            .map_err(|err| format!("ip addr show output is not UTF-8: {err}"))?;
        for line in stdout.lines() {
            let parts: Vec<&str> = line.split_whitespace().collect();
            for window in parts.windows(2) {
                if window[0] == "inet" {
                    return Ok(window[1].to_string());
                }
            }
        }
        Err(format!("no IPv4 address found on interface {iface:?}"))
    }

    fn wg_dump(&self, iface: &str) -> Result<String, String> {
        command_stdout("wg", &["show", iface, "dump"])
    }

    fn ipset_list(&self, set_name: &str) -> Result<String, String> {
        command_stdout("ipset", &["list", set_name, "-o", "save"])
    }

    fn ipset_create(
        &self,
        set_name: &str,
        set_type: &str,
        with_comment: bool,
    ) -> Result<(), String> {
        let mut args = vec!["create", set_name, set_type, "family", "inet", "-exist"];
        if with_comment {
            args.push("comment");
        }
        command_ok("ipset", &args)
    }

    fn ipset_add(&self, set_name: &str, entry: &str, comment: &str) -> Result<(), String> {
        let mut args = vec!["add", set_name, entry];
        if !comment.is_empty() {
            args.extend(["comment", comment]);
        }
        command_ok("ipset", &args)
    }

    fn ipset_del(&self, set_name: &str, entry: &str) -> Result<(), String> {
        command_ok("ipset", &["del", set_name, entry])
    }

    fn wg_set_peer(&self, iface: &str, pub_key: &str, allowed_ip: &str) -> Result<(), String> {
        let allowed_ip = format!("{allowed_ip}/32");
        command_ok(
            "wg",
            &["set", iface, "peer", pub_key, "allowed-ips", &allowed_ip],
        )
    }

    fn wg_del_peer(&self, iface: &str, pub_key: &str) -> Result<(), String> {
        command_ok("wg", &["set", iface, "peer", pub_key, "remove"])
    }

    fn wg_gen_key(&self) -> Result<String, String> {
        command_stdout("wg", &["genkey"]).map(|key| key.trim().to_string())
    }

    fn wg_pub_key(&self, private_key: &str) -> Result<String, String> {
        let mut child = std::process::Command::new("wg")
            .arg("pubkey")
            .stdin(std::process::Stdio::piped())
            .stdout(std::process::Stdio::piped())
            .stderr(std::process::Stdio::piped())
            .spawn()
            .map_err(|err| format!("run wg pubkey: {err}"))?;
        {
            let stdin = child
                .stdin
                .as_mut()
                .ok_or_else(|| "open wg pubkey stdin".to_string())?;
            stdin
                .write_all(private_key.as_bytes())
                .and_then(|_| stdin.write_all(b"\n"))
                .map_err(|err| format!("write wg pubkey stdin: {err}"))?;
        }
        let output = child
            .wait_with_output()
            .map_err(|err| format!("wait wg pubkey: {err}"))?;
        if !output.status.success() {
            return Err(command_stderr("wg pubkey", &output));
        }
        String::from_utf8(output.stdout)
            .map(|key| key.trim().to_string())
            .map_err(|err| format!("wg pubkey output is not UTF-8: {err}"))
    }
}

fn command_stdout(program: &str, args: &[&str]) -> Result<String, String> {
    let output = std::process::Command::new(program)
        .args(args)
        .output()
        .map_err(|err| format!("run {program}: {err}"))?;
    if !output.status.success() {
        return Err(command_stderr(program, &output));
    }
    String::from_utf8(output.stdout).map_err(|err| format!("{program} output is not UTF-8: {err}"))
}

fn command_ok(program: &str, args: &[&str]) -> Result<(), String> {
    let output = std::process::Command::new(program)
        .args(args)
        .output()
        .map_err(|err| format!("run {program}: {err}"))?;
    if !output.status.success() {
        return Err(command_stderr(program, &output));
    }
    Ok(())
}

fn command_stderr(program: &str, output: &std::process::Output) -> String {
    let stderr = String::from_utf8_lossy(&output.stderr);
    let stderr = stderr.trim();
    if stderr.is_empty() {
        format!("{program} exited with {}", output.status)
    } else {
        stderr.to_string()
    }
}
