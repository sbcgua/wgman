# Port-Limited VM Access

Current access control is based on two levels:

- all-access users: source VPN IP is in the all-access ipset;
- matrix users: source VPN IP and destination VM IP pair is in the matrix ipset.

This controls who may reach which VM, but not which services on that VM may be reached. If a user is allowed to a VM, the firewall rule can only allow or deny traffic to the VM as a whole unless ports are hardcoded in iptables rules outside the managed access data.

The idea is to add another concept to the confguration - "resource" (or maybe "service" - discussible) - which would be a combination of vm and port (or ports). A user then may have access to VMs and/or resources.

Tehcnically, add a separate port-aware ipset for service-limited access, preferably:

- `hash:ip,net,port` for TCP/UDP rules where the source VPN IP, destination VM IP/CIDR, and destination port are matched together.
- the set must be referred in the `env.yaml` as `port_matrix`
- the current `matrix` attribute should be renamed to `ip_matrix`

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

- `hash:ip,port,ip` keeps the policy in ipset data instead of baking service lists into the hook script (`/share/.../wgman-firewall-hook.template`).
- Separate TCP and UDP iptables rules are still needed because iptables must select the packet protocol before the destination port match can be meaningful.
- ICMP does not fit a port-based set. ICMP needs an experiment if such an ipset may be used as ip:ip hash, to allow ICMP to selected VM. Or introduce a separate protocol-specific rule/set later.
- The hook script should stay generic: it should wire firewall rules for the configured sets, while access entries remain managed in the `db.yaml`.

Suggested config shape (`db.yaml`):

- Add a section `resources`, that would be a combination of a known vm and port list
- resource name should allow `@` symbol - it would make convenient to name resources like `service@vm` if needed
- resources may be referred from access in the same manner as a vm. Thus, the it's name must be unique over both resources and VMs - e.g. there may be no resource `sandbox` and vm `sandbox`
- port list can be single digit, or an array. Port may have prefix for TCP or UDP (`tcp:`, `udp:`). Port without prefix is supposed to be TCP (this is supposedly the default behavior of the ipset itself - manpage: _"The hash:ip,port,ip set type uses a hash to store IP address, port number and a second IP address triples. The port number is interpreted together with a protocol (default TCP) and zero protocol number cannot be used."_). Example config model below. Missing port is an error.
- a resource may have optional comment

```yaml
users:
  ...
vms:
  sandbox: 192.168.122.190
resources:
  ssh@sandbox:
    vm: sandbox
    ports: 22
    comment: access to SSH
  web@sandbox:
    vm: sandbox
    ports: [80, 443, 8080]
  web2@sandbox:
    vm: sandbox
    ports:
      - 80
      - 443
      - 8080
  xxx@sandbox:
    vm: sandbox
    ports:
      - tcp:80
      - udp:53
access:
  user1:
    - sandbox # full VM access -> add to matrix
  user2:
    - ssh@sandbox # access to service only
```

Other considerations:

- the semantics of the wgman commands supposed to be compatible without extra changes. E.g. `wgman mod user1 +ssh@sandbox` would add this resource for the user
- `wgman-firewall-hook.template` must be updated with the final version if `iptables` call
- init-ipsets should create the new set as well
- check and deploy logic should be concentrated in the `check` and `deploy` files respectively, the rest of the commands should delegate the validation and application of rules to them (as it is now). Presumably, VMs and Resources look similar to commands, so their maintenance will mainly hapen internally in `check`, `deploy` and `db` logic areas.
