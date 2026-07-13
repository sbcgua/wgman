use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::Path;

pub fn render_client_config(
    template_path: impl AsRef<Path>,
    private_key: &str,
    client_ip: &str,
    server_public_key: &str,
) -> Result<String, String> {
    let data = fs::read_to_string(template_path)
        .map_err(|err| format!("read user.conf.template: {err}"))?;
    let mut out = trim_leading_blank_lines(&strip_comment_lines(&data));
    out = out.replace("$CLIENT_PRIVATE_KEY", private_key);
    out = out.replace("$CLIENT_VPN_IP", client_ip);
    out = out.replace("$SERVER_PUBLIC_KEY", server_public_key);
    Ok(out)
}

pub fn write_client_config_no_overwrite(
    path: impl AsRef<Path>,
    content: &str,
) -> Result<(), String> {
    let path = path.as_ref();
    let mut options = OpenOptions::new();
    options.write(true).create_new(true);
    set_mode_0600(&mut options);
    let mut file = options
        .open(path)
        .map_err(|err| format!("create {}: {err}", path.display()))?;
    file.write_all(content.as_bytes())
        .map_err(|err| format!("write {}: {err}", path.display()))?;
    file.sync_all()
        .map_err(|err| format!("sync {}: {err}", path.display()))?;
    Ok(())
}

fn strip_comment_lines(value: &str) -> String {
    let mut out = String::new();
    for line in value.split_inclusive('\n') {
        let without_newline = line.trim_end_matches(['\r', '\n']);
        if without_newline.trim_start().starts_with('#') {
            continue;
        }
        out.push_str(line);
    }
    if !value.ends_with('\n') {
        let last = value.rsplit('\n').next().unwrap_or(value);
        if !last.trim_start().starts_with('#') && !out.ends_with(last) {
            out.push_str(last);
        }
    }
    out
}

fn trim_leading_blank_lines(value: &str) -> String {
    let mut out = value;
    loop {
        let Some(next) = out.find('\n') else {
            return if out.trim().is_empty() {
                String::new()
            } else {
                out.to_string()
            };
        };
        let line = &out[..=next];
        if !line.trim().is_empty() {
            return out.to_string();
        }
        out = &out[next + 1..];
    }
}

fn set_mode_0600(options: &mut OpenOptions) {
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }
    #[cfg(not(unix))]
    {
        let _ = options;
    }
}
