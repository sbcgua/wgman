use std::io::{self, Write};
use std::time::{SystemTime, UNIX_EPOCH};

pub const ANSI_RESET: &str = "\x1b[0m";
pub const ANSI_GREY: &str = "\x1b[90m";
pub const ANSI_RED: &str = "\x1b[31m";
pub const ANSI_GREEN: &str = "\x1b[32m";
pub const ANSI_YELLOW: &str = "\x1b[33m";
pub const ANSI_CYAN: &str = "\x1b[36m";
pub const ANSI_LIGHT_BLUE: &str = "\x1b[94m";
pub const ANSI_PINK: &str = "\x1b[95m";
pub const ANSI_DIM_CYAN: &str = "\x1b[2;36m";

pub fn color_grey(value: &str) -> String {
    color(ANSI_GREY, value)
}

pub fn color_red(value: &str) -> String {
    color(ANSI_RED, value)
}

pub fn color_green(value: &str) -> String {
    color(ANSI_GREEN, value)
}

pub fn color_yellow(value: &str) -> String {
    color(ANSI_YELLOW, value)
}

pub fn color_cyan(value: &str) -> String {
    color(ANSI_CYAN, value)
}

pub fn color_light_blue(value: &str) -> String {
    color(ANSI_LIGHT_BLUE, value)
}

pub fn color_pink(value: &str) -> String {
    color(ANSI_PINK, value)
}

pub fn color_dim_cyan(value: &str) -> String {
    color(ANSI_DIM_CYAN, value)
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct TableCell {
    pub plain: String,
    pub display: String,
}

impl TableCell {
    pub fn plain(value: impl Into<String>) -> Self {
        let value = value.into();
        Self {
            plain: value.clone(),
            display: value,
        }
    }

    pub fn new(plain: impl Into<String>, display: impl Into<String>) -> Self {
        Self {
            plain: plain.into(),
            display: display.into(),
        }
    }
}

pub fn write_table<W: Write>(
    writer: &mut W,
    headers: &[&str],
    rows: &[Vec<TableCell>],
) -> io::Result<()> {
    let mut widths: Vec<usize> = headers.iter().map(|header| header.len()).collect();
    for row in rows {
        for (index, cell) in row.iter().enumerate() {
            if let Some(width) = widths.get_mut(index) {
                *width = (*width).max(cell.plain.len());
            }
        }
    }

    let header_cells: Vec<TableCell> = headers.iter().copied().map(TableCell::plain).collect();
    write_table_row(writer, &header_cells, &widths)?;
    for row in rows {
        write_table_row(writer, row, &widths)?;
    }
    Ok(())
}

pub fn format_bytes(bytes: i64) -> String {
    const KB: i64 = 1024;
    const MB: i64 = 1024 * KB;
    const GB: i64 = 1024 * MB;

    if bytes < KB {
        format!("{bytes}B")
    } else if bytes < MB {
        format!("{:.2}Kb", bytes as f64 / KB as f64)
    } else if bytes < GB {
        format!("{:.2}Mb", bytes as f64 / MB as f64)
    } else {
        format!("{:.2}Gb", bytes as f64 / GB as f64)
    }
}

pub fn format_bytes_color(bytes: i64, enabled: bool) -> String {
    let plain = format_bytes(bytes);
    if !enabled {
        return plain;
    }
    if bytes == 0 {
        return color_grey(&plain);
    }

    let unit_start = plain
        .find(|ch: char| !(ch.is_ascii_digit() || ch == '.'))
        .unwrap_or(plain.len());
    if unit_start == plain.len() {
        plain
    } else {
        format!(
            "{}{}",
            &plain[..unit_start],
            color_dim_cyan(&plain[unit_start..])
        )
    }
}

pub fn format_handshake(timestamp: i64, now: SystemTime) -> String {
    if timestamp == 0 {
        return "never".to_string();
    }
    let (days, hours, minutes, seconds) = handshake_age_parts(timestamp, now);
    match (days, hours, minutes) {
        (days, _, _) if days > 0 => format!("{days}d{hours}h{minutes}m{seconds}s"),
        (_, hours, _) if hours > 0 => format!("{hours}h{minutes}m{seconds}s"),
        (_, _, minutes) if minutes > 0 => format!("{minutes}m{seconds}s"),
        _ => format!("{seconds}s"),
    }
}

pub fn format_handshake_color(timestamp: i64, now: SystemTime, enabled: bool) -> String {
    if !enabled {
        return format_handshake(timestamp, now);
    }
    if timestamp == 0 {
        return color_grey("never");
    }

    let (days, hours, minutes, seconds) = handshake_age_parts(timestamp, now);
    match (days, hours, minutes) {
        (days, _, _) if days > 0 => {
            format!(
                "{}{hours}h{}{seconds}s",
                color_dim_cyan(&format!("{days}d")),
                color_dim_cyan(&format!("{minutes}m"))
            )
        }
        (_, hours, _) if hours > 0 => {
            format!(
                "{hours}h{}{seconds}s",
                color_dim_cyan(&format!("{minutes}m"))
            )
        }
        (_, _, minutes) if minutes > 0 => {
            format!("{}{seconds}s", color_dim_cyan(&format!("{minutes}m")))
        }
        _ => format!("{seconds}s"),
    }
}

pub fn handshake_age_parts(timestamp: i64, now: SystemTime) -> (i64, i64, i64, i64) {
    let now_seconds = now
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs() as i64)
        .unwrap_or(0);
    let mut age = now_seconds - timestamp;
    if age < 0 {
        age = 0;
    }

    let seconds = age % 60;
    age /= 60;
    let minutes = age % 60;
    age /= 60;
    let hours = age % 24;
    let days = age / 24;
    (days, hours, minutes, seconds)
}

fn color(code: &str, value: &str) -> String {
    format!("{code}{value}{ANSI_RESET}")
}

fn write_table_row<W: Write>(
    writer: &mut W,
    row: &[TableCell],
    widths: &[usize],
) -> io::Result<()> {
    for (index, cell) in row.iter().enumerate() {
        if index == row.len() - 1 {
            writeln!(writer, "{}", cell.display)?;
            return Ok(());
        }
        write!(writer, "{}", cell.display)?;
        let padding = widths[index].saturating_sub(cell.plain.len()) + 2;
        write!(writer, "{:padding$}", "")?;
    }
    writeln!(writer)
}
