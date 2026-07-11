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
