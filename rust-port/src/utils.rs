use std::io::{BufRead, Write};
use std::net::Ipv4Addr;

pub fn case_fold_ascii(value: &str) -> String {
    value.to_ascii_lowercase()
}

pub fn sorted_keys<T>(values: &std::collections::HashMap<String, T>) -> Vec<String> {
    let mut keys: Vec<String> = values.keys().cloned().collect();
    keys.sort();
    keys
}

pub fn reject_yaml_tags(value: &serde_yaml::Value, filename: &str) -> Result<(), String> {
    match value {
        serde_yaml::Value::Sequence(items) => {
            for item in items {
                reject_yaml_tags(item, filename)?;
            }
        }
        serde_yaml::Value::Mapping(items) => {
            for (key, item) in items {
                reject_yaml_tags(key, filename)?;
                reject_yaml_tags(item, filename)?;
            }
        }
        serde_yaml::Value::Tagged(tagged) => {
            return Err(format!(
                "{filename}: YAML tags are not supported ({})",
                tagged.tag
            ));
        }
        serde_yaml::Value::Null
        | serde_yaml::Value::Bool(_)
        | serde_yaml::Value::Number(_)
        | serde_yaml::Value::String(_) => {}
    }
    Ok(())
}

pub fn endpoint_host(endpoint: &str) -> String {
    if endpoint == "(none)" {
        return endpoint.to_string();
    }

    if let Some(rest) = endpoint.strip_prefix('[') {
        if let Some((host, port)) = rest.split_once("]:") {
            if !host.is_empty() && !port.is_empty() {
                return host.to_string();
            }
        }
        return endpoint.to_string();
    }

    if endpoint.matches(':').count() == 1 {
        if let Some((host, port)) = endpoint.rsplit_once(':') {
            if !host.is_empty() && !port.is_empty() {
                return host.to_string();
            }
        }
    }

    endpoint.to_string()
}

pub fn split_lines(value: &str) -> Vec<String> {
    value
        .split('\n')
        .filter_map(|line| {
            let line = line.strip_suffix('\r').unwrap_or(line);
            (!line.is_empty()).then(|| line.to_string())
        })
        .collect()
}

pub fn confirm_action<R: BufRead, W: Write>(input: &mut R, output: &mut W, prompt: &str) -> bool {
    let _ = write!(output, "{prompt}");
    let _ = output.flush();

    let mut answer = String::new();
    if input.read_line(&mut answer).is_err() {
        return false;
    }
    matches!(answer.trim(), "y" | "Y")
}

pub fn is_valid_ipv4_or_cidr(value: &str) -> bool {
    value.parse::<Ipv4Addr>().is_ok() || parse_ipv4_cidr(value).is_ok()
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct IPv4Cidr {
    pub ip: Ipv4Addr,
    pub network: Ipv4Addr,
    pub prefix: u8,
}

impl IPv4Cidr {
    pub fn network_string(&self) -> String {
        format!("{}/{}", self.network, self.prefix)
    }
}

pub fn parse_ipv4_cidr(subnet: &str) -> Result<IPv4Cidr, String> {
    let Some((ip, prefix)) = subnet.split_once('/') else {
        return Err(format!("invalid interface subnet {subnet:?}"));
    };
    let ip: Ipv4Addr = ip
        .parse()
        .map_err(|_| format!("invalid interface subnet {subnet:?}"))?;
    let prefix: u8 = prefix
        .parse()
        .map_err(|_| format!("invalid interface subnet {subnet:?}"))?;
    if prefix > 32 {
        return Err(format!("interface subnet {subnet:?} is not IPv4"));
    }

    let mask = if prefix == 0 {
        0
    } else {
        u32::MAX << (32 - u32::from(prefix))
    };
    let network = uint32_to_ipv4(ipv4_to_u32(ip) & mask);
    Ok(IPv4Cidr {
        ip,
        network,
        prefix,
    })
}

pub fn ipv4_to_u32(ip: Ipv4Addr) -> u32 {
    u32::from(ip)
}

pub fn uint32_to_ipv4(value: u32) -> Ipv4Addr {
    Ipv4Addr::from(value)
}
