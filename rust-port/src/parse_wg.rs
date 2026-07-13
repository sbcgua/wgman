use std::net::IpAddr;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct WGDumpResult {
    pub server_pub_key: String,
    pub peers: Vec<WGPeer>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct WGPeer {
    pub public_key: String,
    pub preshared_key: String,
    pub endpoint: String,
    pub allowed_ip: String,
    pub latest_handshake: i64,
    pub rx_bytes: i64,
    pub tx_bytes: i64,
    pub keepalive: String,
}

pub fn parse_wg_dump(output: &str) -> Result<WGDumpResult, String> {
    let lines = crate::utils::split_lines(output);
    if lines.is_empty() {
        return Err("wg dump: empty output".to_string());
    }

    let server_fields: Vec<&str> = lines[0].split('\t').collect();
    if server_fields.len() < 4 {
        return Err(format!(
            "wg dump: server line has {} fields, want >=4: {:?}",
            server_fields.len(),
            lines[0]
        ));
    }

    let mut result = WGDumpResult {
        server_pub_key: server_fields[1].to_string(),
        peers: Vec::new(),
    };

    for (index, line) in lines.iter().skip(1).enumerate() {
        let peer = parse_wg_peer_line(line)
            .map_err(|err| format!("wg dump: peer line {}: {err}", index + 2))?;
        result.peers.push(peer);
    }

    Ok(result)
}

pub fn parse_wg_peer_line(line: &str) -> Result<WGPeer, String> {
    let fields: Vec<&str> = line.split('\t').collect();
    if fields.len() < 8 {
        return Err(format!(
            "peer line has {} fields, want >=8: {:?}",
            fields.len(),
            line
        ));
    }

    let latest_handshake = parse_i64_field(fields[4], "latest-handshake")?;
    let rx_bytes = parse_i64_field(fields[5], "transfer-rx")?;
    let tx_bytes = parse_i64_field(fields[6], "transfer-tx")?;
    let allowed_ip = normalize_wg_allowed_ip(fields[3])
        .map_err(|err| format!("normalize allowed-ip {:?}: {err}", fields[3]))?;

    Ok(WGPeer {
        public_key: fields[0].to_string(),
        preshared_key: fields[1].to_string(),
        endpoint: fields[2].to_string(),
        allowed_ip,
        latest_handshake,
        rx_bytes,
        tx_bytes,
        keepalive: fields[7].to_string(),
    })
}

pub fn normalize_wg_allowed_ip(value: &str) -> Result<String, String> {
    if value.contains(',') {
        return Err(format!("unexpected multiple allowed-ips {value:?}"));
    }

    if let Some((ip, prefix)) = value.split_once('/') {
        let parsed_ip: IpAddr = ip
            .parse()
            .map_err(|err| format!("invalid allowed-ip CIDR {value:?}: {err}"))?;
        let prefix: u8 = prefix
            .parse()
            .map_err(|err| format!("invalid allowed-ip CIDR {value:?}: {err}"))?;
        match parsed_ip {
            IpAddr::V4(ipv4) if prefix == 32 => Ok(ipv4.to_string()),
            IpAddr::V4(_) if prefix <= 32 => Ok(value.to_string()),
            IpAddr::V6(_) if prefix <= 128 => Ok(value.to_string()),
            IpAddr::V4(_) | IpAddr::V6(_) => {
                Err(format!("invalid allowed-ip CIDR {value:?}: invalid prefix"))
            }
        }
    } else {
        match value.parse::<IpAddr>() {
            Ok(_) => Ok(value.to_string()),
            Err(_) => Err(format!("invalid allowed-ip {value:?}")),
        }
    }
}

fn parse_i64_field(value: &str, name: &str) -> Result<i64, String> {
    value
        .parse::<i64>()
        .map_err(|err| format!("parse {name} {value:?}: {err}"))
}
