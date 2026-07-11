use std::collections::HashMap;
use std::io::{BufReader, Cursor};
use std::time::{Duration, UNIX_EPOCH};

use wgman_rs::format::{
    color_grey, color_red, format_bytes, format_bytes_color, format_handshake,
    format_handshake_color, handshake_age_parts, write_table, TableCell, ANSI_DIM_CYAN, ANSI_GREY,
    ANSI_RESET,
};
use wgman_rs::parse_ipset::parse_ipset;
use wgman_rs::parse_wg::{normalize_wg_allowed_ip, parse_wg_dump};
use wgman_rs::utils::{
    case_fold_ascii, confirm_action, endpoint_host, ipv4_to_u32, is_valid_ipv4_or_cidr,
    parse_ipv4_cidr, sorted_keys, split_lines, uint32_to_ipv4,
};

const WG_DUMP_VALID: &str = concat!(
    "SERVER_PRIV_KEY=\tSERVER_PUB_KEY=\t51820\toff\n",
    "ALICE_PUB=\t(none)\t192.168.1.100:50001\t10.8.0.10/32\t1748000000\t102400\t204800\toff\n",
    "BOB_PUB=\t(none)\t(none)\t10.8.0.15/32\t0\t0\t0\toff\n",
);

fn assert_error_contains<T: std::fmt::Debug>(result: Result<T, String>, want: &str) {
    let error = result.expect_err("expected error");
    assert!(
        error.contains(want),
        "error {error:?} does not contain {want:?}"
    );
}

#[test]
fn parse_wg_dump_valid() {
    let result = parse_wg_dump(WG_DUMP_VALID).unwrap();

    assert_eq!(result.server_pub_key, "SERVER_PUB_KEY=");
    assert_eq!(result.peers.len(), 2);

    let alice = &result.peers[0];
    assert_eq!(alice.public_key, "ALICE_PUB=");
    assert_eq!(alice.allowed_ip, "10.8.0.10");
    assert_eq!(alice.endpoint, "192.168.1.100:50001");
    assert_eq!(alice.latest_handshake, 1_748_000_000);
    assert_eq!(alice.rx_bytes, 102_400);
    assert_eq!(alice.tx_bytes, 204_800);
    assert_eq!(alice.keepalive, "off");

    let bob = &result.peers[1];
    assert_eq!(bob.endpoint, "(none)");
    assert_eq!(bob.allowed_ip, "10.8.0.15");
    assert_eq!(bob.latest_handshake, 0);
}

#[test]
fn parse_wg_dump_no_peers() {
    let result = parse_wg_dump("SERVER_PRIV=\tSERVER_PUB=\t51820\toff\n").unwrap();

    assert_eq!(result.server_pub_key, "SERVER_PUB=");
    assert!(result.peers.is_empty());
}

#[test]
fn parse_wg_dump_errors() {
    let cases = [
        ("empty output", "", "empty output"),
        (
            "server line too short",
            "A\tB\t51820\n",
            "server line has 3 fields",
        ),
        (
            "peer line too few fields",
            "PRIV=\tPUB=\t51820\toff\nPEER_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\t0\n",
            "peer line has 7 fields",
        ),
        (
            "peer non-numeric handshake",
            "PRIV=\tPUB=\t51820\toff\nPEER_PUB=\t(none)\t(none)\t10.8.0.5/32\tBAD\t0\t0\toff\n",
            "parse latest-handshake",
        ),
        (
            "peer non-numeric rx",
            "PRIV=\tPUB=\t51820\toff\nPEER_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\tBAD\t0\toff\n",
            "parse transfer-rx",
        ),
        (
            "peer non-numeric tx",
            "PRIV=\tPUB=\t51820\toff\nPEER_PUB=\t(none)\t(none)\t10.8.0.5/32\t0\t0\tBAD\toff\n",
            "parse transfer-tx",
        ),
        (
            "peer multiple allowed-ips",
            "PRIV=\tPUB=\t51820\toff\nPEER_PUB=\t(none)\t(none)\t10.8.0.5/32,10.8.0.6/32\t0\t0\t0\toff\n",
            "unexpected multiple allowed-ips",
        ),
        (
            "peer invalid allowed-ip",
            "PRIV=\tPUB=\t51820\toff\nPEER_PUB=\t(none)\t(none)\tnotanip\t0\t0\t0\toff\n",
            "invalid allowed-ip",
        ),
    ];

    for (name, input, want) in cases {
        assert_error_contains(parse_wg_dump(input), want);
        let _ = name;
    }
}

#[test]
fn normalize_wg_allowed_ip_cases() {
    let cases = [
        ("10.8.0.5/32", Ok("10.8.0.5")),
        ("10.8.0.5", Ok("10.8.0.5")),
        ("10.8.0.0/24", Ok("10.8.0.0/24")),
        ("10.8.0.5/32,10.8.0.6/32", Err("unexpected multiple")),
        ("notanip", Err("invalid allowed-ip")),
    ];

    for (input, want) in cases {
        match want {
            Ok(want) => assert_eq!(normalize_wg_allowed_ip(input).unwrap(), want),
            Err(want) => assert_error_contains(normalize_wg_allowed_ip(input), want),
        }
    }
}

#[test]
fn parse_ipset_all_access_set() {
    let input = concat!(
        "create wg_allow_all hash:ip family inet hashsize 1024 maxelem 65536\n",
        "add wg_allow_all 10.8.0.5\n",
    );

    let parsed = parse_ipset(input).unwrap();
    assert_eq!(parsed.set_name, "wg_allow_all");
    assert_eq!(parsed.set_type, "hash:ip");
    assert_eq!(parsed.entries.len(), 1);
    assert_eq!(parsed.entries[0].entry, "10.8.0.5");
    assert_eq!(parsed.entries[0].comment, "");
}

#[test]
fn parse_ipset_matrix_set_and_comments() {
    let input = concat!(
        "create wg_allow_matrix hash:net,net family inet hashsize 1024 maxelem 65536 comment\n",
        "add wg_allow_matrix 10.8.0.10,192.168.122.100 comment \"alice -> sandbox\"\n",
        "add wg_allow_matrix 10.8.0.15,192.168.122.100 comment \"bob -> sandbox\"\n",
        "add wg_allow_matrix 10.8.0.15,192.168.122.101 comment \"bob -> mailvm\"\n",
    );

    let parsed = parse_ipset(input).unwrap();
    assert_eq!(parsed.set_name, "wg_allow_matrix");
    assert_eq!(parsed.set_type, "hash:net,net");
    assert_eq!(parsed.entries.len(), 3);
    assert_eq!(parsed.entries[0].entry, "10.8.0.10,192.168.122.100");
    assert_eq!(parsed.entries[0].comment, "alice -> sandbox");
    assert_eq!(parsed.entries[2].entry, "10.8.0.15,192.168.122.101");
    assert_eq!(parsed.entries[2].comment, "bob -> mailvm");
}

#[test]
fn parse_ipset_port_matrix_and_empty_set() {
    let empty = parse_ipset("create wg_allow_all hash:ip family inet\n").unwrap();
    assert!(empty.entries.is_empty());

    let input = concat!(
        "create wg_allow_matrix_ports hash:ip,port,ip family inet comment\n",
        "add wg_allow_matrix_ports 10.8.0.10,tcp:22,192.168.122.100 comment \"alice -> ssh@sandbox tcp/22\"\n",
    );
    let parsed = parse_ipset(input).unwrap();
    assert_eq!(parsed.set_type, "hash:ip,port,ip");
    assert_eq!(parsed.entries[0].entry, "10.8.0.10,tcp:22,192.168.122.100");
    assert_eq!(parsed.entries[0].comment, "alice -> ssh@sandbox tcp/22");
}

#[test]
fn parse_ipset_comment_with_spaces_and_without_comment() {
    let input = concat!(
        "create mySet hash:net,net family inet\n",
        "add mySet 10.0.0.1,10.0.0.2 comment \"user name -> vm name\"\n",
        "add mySet 10.0.0.3,10.0.0.4\n",
    );

    let parsed = parse_ipset(input).unwrap();
    assert_eq!(parsed.entries[0].comment, "user name -> vm name");
    assert_eq!(parsed.entries[1].comment, "");
}

#[test]
fn parse_ipset_errors() {
    let cases = [
        (
            "unexpected line",
            "create mySet hash:net family inet\nfoo bar baz\n",
            "unexpected line",
        ),
        ("add line too short", "add mySet\n", "add line too short"),
        (
            "unexpected trailing content",
            "create mySet hash:net family inet\nadd mySet 10.0.0.1 garbage here\n",
            "unexpected trailing content",
        ),
        (
            "add before create",
            "add mySet 10.0.0.1\n",
            "add line before create line",
        ),
        (
            "duplicate create line",
            "create mySet hash:ip family inet\ncreate mySet hash:ip family inet\n",
            "duplicate create line",
        ),
        (
            "add set name mismatch",
            "create mySet hash:ip family inet\nadd otherSet 10.0.0.1\n",
            "does not match create set name",
        ),
    ];

    for (name, input, want) in cases {
        assert_error_contains(parse_ipset(input), want);
        let _ = name;
    }
}

#[test]
fn utils_match_go_behavior() {
    let input = HashMap::from([
        ("charlie".to_string(), 3),
        ("alice".to_string(), 1),
        ("bob".to_string(), 2),
    ]);
    assert_eq!(sorted_keys(&input), ["alice", "bob", "charlie"]);

    assert!(is_valid_ipv4_or_cidr("10.8.0.5"));
    assert!(is_valid_ipv4_or_cidr("10.8.0.0/24"));
    assert!(!is_valid_ipv4_or_cidr("not-an-ip"));
    assert!(!is_valid_ipv4_or_cidr("2001:db8::1"));
    assert!(!is_valid_ipv4_or_cidr("2001:db8::/32"));

    let cidr = parse_ipv4_cidr("10.8.0.1/24").unwrap();
    assert_eq!(cidr.ip.to_string(), "10.8.0.1");
    assert_eq!(cidr.network_string(), "10.8.0.0/24");
    assert!(parse_ipv4_cidr("not-a-cidr").is_err());
    assert!(parse_ipv4_cidr("2001:db8::1/64").is_err());

    let ip = "10.8.0.16".parse().unwrap();
    let n = ipv4_to_u32(ip);
    assert_eq!(n, 0x0a080010);
    assert_eq!(uint32_to_ipv4(n).to_string(), "10.8.0.16");

    assert_eq!(case_fold_ascii("Alice_BOB-42"), "alice_bob-42");
    assert_eq!(endpoint_host("(none)"), "(none)");
    assert_eq!(endpoint_host("192.168.1.100:50001"), "192.168.1.100");
    assert_eq!(endpoint_host("10.0.0.1:51820"), "10.0.0.1");
    assert_eq!(
        endpoint_host("myhost.example.com:12345"),
        "myhost.example.com"
    );
    assert_eq!(endpoint_host("[2001:db8::1]:51820"), "2001:db8::1");
    assert_eq!(endpoint_host("2001:db8::1"), "2001:db8::1");
    assert_eq!(split_lines("a\r\n\nb\n"), ["a", "b"]);
}

#[test]
fn confirm_action_accepts_only_single_y() {
    let cases = [
        ("lower yes", "y\n", true),
        ("upper yes trimmed", " Y \n", true),
        ("word yes rejected", "yes\n", false),
        ("default no", "\n", false),
        ("eof", "", false),
    ];

    for (name, input, want) in cases {
        let mut input = BufReader::new(Cursor::new(input.as_bytes()));
        let mut output = Vec::new();
        let got = confirm_action(&mut input, &mut output, "Continue? [y/N] ");
        assert_eq!(got, want, "{name}");
        assert_eq!(
            String::from_utf8(output).unwrap(),
            "Continue? [y/N] ",
            "{name}"
        );
    }
}

#[test]
fn format_bytes_cases() {
    let cases = [
        (0, "0B"),
        (512, "512B"),
        (1023, "1023B"),
        (1024, "1.00Kb"),
        (1536, "1.50Kb"),
        (1024 * 1024, "1.00Mb"),
        (2_170_552, "2.07Mb"),
        (1024 * 1024 * 1024, "1.00Gb"),
    ];

    for (bytes, want) in cases {
        assert_eq!(format_bytes(bytes), want);
    }
}

#[test]
fn write_table_preserves_visible_alignment() {
    let rows = vec![
        vec![
            TableCell::new("bob", color_grey("bob")),
            TableCell::plain("ok"),
        ],
        vec![
            TableCell::plain("alexandra"),
            TableCell::new("needs-work", color_red("needs-work")),
        ],
    ];

    let mut out = Vec::new();
    write_table(&mut out, &["NAME", "STATUS"], &rows).unwrap();

    let want = format!(
        "NAME       STATUS\n{}        ok\nalexandra  {}\n",
        color_grey("bob"),
        color_red("needs-work")
    );
    assert_eq!(String::from_utf8(out).unwrap(), want);
}

#[test]
fn format_bytes_color_cases() {
    let cases = [
        ("disabled", 1024, false, "1.00Kb".to_string()),
        ("zero", 0, true, format!("{ANSI_GREY}0B{ANSI_RESET}")),
        (
            "bytes unit",
            512,
            true,
            format!("512{ANSI_DIM_CYAN}B{ANSI_RESET}"),
        ),
        (
            "kilobytes unit",
            1024,
            true,
            format!("1.00{ANSI_DIM_CYAN}Kb{ANSI_RESET}"),
        ),
        (
            "megabytes unit",
            1024 * 1024,
            true,
            format!("1.00{ANSI_DIM_CYAN}Mb{ANSI_RESET}"),
        ),
        (
            "gigabytes unit",
            1024 * 1024 * 1024,
            true,
            format!("1.00{ANSI_DIM_CYAN}Gb{ANSI_RESET}"),
        ),
    ];

    for (name, bytes, enabled, want) in cases {
        assert_eq!(format_bytes_color(bytes, enabled), want, "{name}");
    }
}

#[test]
fn format_handshake_cases() {
    let base = 1_748_000_000;
    let now = UNIX_EPOCH + Duration::from_secs(base as u64);
    let cases = [
        (0, "never"),
        (base - 30, "30s"),
        (base - 90, "1m30s"),
        (base - 3661, "1h1m1s"),
        (base - 258_520, "2d23h48m40s"),
    ];

    for (timestamp, want) in cases {
        assert_eq!(format_handshake(timestamp, now), want);
    }

    assert_eq!(
        format_handshake(2000, UNIX_EPOCH + Duration::from_secs(1000)),
        "0s"
    );
    assert_eq!(handshake_age_parts(base - 258_520, now), (2, 23, 48, 40));
}

#[test]
fn format_handshake_color_cases() {
    let base = 1_748_000_000;
    let now = UNIX_EPOCH + Duration::from_secs(base as u64);

    assert_eq!(format_handshake_color(0, now, false), "never");
    assert_eq!(
        format_handshake_color(0, now, true),
        format!("{ANSI_GREY}never{ANSI_RESET}")
    );
    assert_eq!(
        format_handshake_color(base - 90, now, true),
        format!("{ANSI_DIM_CYAN}1m{ANSI_RESET}30s")
    );
    assert_eq!(
        format_handshake_color(base - 3661, now, true),
        format!("1h{ANSI_DIM_CYAN}1m{ANSI_RESET}1s")
    );
    assert_eq!(
        format_handshake_color(base - 258_520, now, true),
        format!("{ANSI_DIM_CYAN}2d{ANSI_RESET}23h{ANSI_DIM_CYAN}48m{ANSI_RESET}40s")
    );
}
