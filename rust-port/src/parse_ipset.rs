#[derive(Clone, Debug, PartialEq, Eq)]
pub struct IPSetEntry {
    pub entry: String,
    pub comment: String,
}

#[derive(Clone, Debug, PartialEq, Eq, Default)]
pub struct ParsedIPSet {
    pub set_name: String,
    pub set_type: String,
    pub entries: Vec<IPSetEntry>,
}

pub fn parse_ipset(output: &str) -> Result<ParsedIPSet, String> {
    let mut result = ParsedIPSet::default();

    for (index, line) in crate::utils::split_lines(output).iter().enumerate() {
        let line_number = index + 1;
        if line.starts_with("create ") {
            if !result.set_name.is_empty() {
                return Err(format!("ipset line {line_number}: duplicate create line"));
            }
            let (set_name, set_type) = parse_ipset_create_line(line)
                .map_err(|err| format!("ipset line {line_number}: {err}"))?;
            result.set_name = set_name;
            result.set_type = set_type;
        } else if line.starts_with("add ") {
            let (entry, add_set_name) = parse_ipset_add_line(line)
                .map_err(|err| format!("ipset line {line_number}: {err}"))?;
            if result.set_name.is_empty() {
                return Err(format!(
                    "ipset line {line_number}: add line before create line"
                ));
            }
            if add_set_name != result.set_name {
                return Err(format!(
                    "ipset line {line_number}: add set name {add_set_name:?} does not match create set name {:?}",
                    result.set_name
                ));
            }
            result.entries.push(entry);
        } else {
            return Err(format!(
                "ipset line {line_number}: unexpected line {line:?}"
            ));
        }
    }

    Ok(result)
}

pub fn parse_ipset_create_line(line: &str) -> Result<(String, String), String> {
    let parts: Vec<&str> = line.split_whitespace().collect();
    if parts.len() < 3 {
        return Err(format!("create line too short: {line:?}"));
    }
    Ok((parts[1].to_string(), parts[2].to_string()))
}

pub fn parse_ipset_add_line(line: &str) -> Result<(IPSetEntry, String), String> {
    let parts: Vec<&str> = line.splitn(4, ' ').collect();
    if parts.len() < 3 {
        return Err(format!("add line too short: {line:?}"));
    }

    let set_name = parts[1].to_string();
    let mut entry = IPSetEntry {
        entry: parts[2].to_string(),
        comment: String::new(),
    };

    if parts.len() < 4 || parts[3].trim().is_empty() {
        return Ok((entry, set_name));
    }

    let rest = parts[3].trim();
    let Some(comment_value) = rest.strip_prefix("comment ") else {
        return Err(format!("unexpected trailing content {rest:?} in: {line:?}"));
    };

    let comment_value = comment_value.trim();
    entry.comment = if comment_value.starts_with('"')
        && comment_value.ends_with('"')
        && comment_value.len() >= 2
    {
        comment_value[1..comment_value.len() - 1].to_string()
    } else {
        comment_value.to_string()
    };

    Ok((entry, set_name))
}
