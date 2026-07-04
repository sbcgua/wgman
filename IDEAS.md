# Ideas

## Port-Limited VM Access

Current access control is based on two levels:

- all-access users: source VPN IP is in the all-access ipset;
- matrix users: source VPN IP and destination VM IP pair is in the matrix ipset.

This controls who may reach which VM, but not which services on that VM may be reached. If a user is allowed to a VM, the firewall rule can only allow or deny traffic to the VM as a whole unless ports are hardcoded in iptables rules outside the managed access data.

Recommended firewall model:

- Keep the existing all-access set as `hash:ip`.
- Keep the existing VM matrix set as `hash:net,net` for broad VM access.
- Add a separate port-aware set for service-limited access, preferably:
  - `hash:ip,net,port` for TCP/UDP rules where the source VPN IP, destination VM IP/CIDR, and destination port are matched together.

Example shape:

```sh
ipset create wg_allow_matrix_ports hash:ip,port,ip family inet comment
ipset add wg_allow_matrix_ports 10.7.0.10,tcp:5432,192.168.122.20 comment "alice -> db1 tcp/5432"
ipset add wg_allow_matrix_ports 10.7.0.11,udp:53,192.168.122.30 comment "bob -> dns1 udp/53"
```

Example iptables rules:

```sh
iptables -A WGMAN_FWD -i wg0 -o virbr0 -p tcp -m set --match-set wg_allow_matrix_ports src,dst,dst -j ACCEPT
iptables -A WGMAN_FWD -i wg0 -o virbr0 -p udp -m set --match-set wg_allow_matrix_ports src,dst,dst -j ACCEPT
```

Notes:

- `hash:ip,port,ip` keeps the policy in ipset data instead of baking service lists into the hook script.
- Separate TCP and UDP iptables rules are still needed because iptables must select the packet protocol before the destination port match can be meaningful.
- ICMP does not fit a port-based set. If ICMP needs per-VM control, keep it in the broad matrix set or introduce a separate protocol-specific rule/set later.
- The hook script should stay generic: it should wire firewall rules for the configured sets, while access entries remain managed elsewhere.
