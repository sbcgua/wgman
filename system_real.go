package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// RealSystem is the production SystemAdapter that invokes real OS commands.
type RealSystem struct{}

func (r *RealSystem) IsRoot() bool {
	return os.Getuid() == 0
}

// InterfaceSubnet returns the first IPv4 CIDR address on iface, e.g. "10.8.0.1/24".
func (r *RealSystem) InterfaceSubnet(iface string) (string, error) {
	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		return "", fmt.Errorf("interface %q: %w", iface, err)
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return "", fmt.Errorf("interface %q addrs: %w", iface, err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil {
			return ipNet.String(), nil
		}
	}
	return "", fmt.Errorf("interface %q has no IPv4 address", iface)
}

func (r *RealSystem) WGDump(iface string) (string, error) {
	out, err := exec.Command("wg", "show", iface, "dump").Output()
	if err != nil {
		return "", fmt.Errorf("wg show %s dump: %w", iface, err)
	}
	return string(out), nil
}

func (r *RealSystem) IPSetList(setname string) (string, error) {
	out, err := exec.Command("ipset", "list", setname, "-o", "save").Output()
	if err != nil {
		return "", fmt.Errorf("ipset list %s: %w", setname, err)
	}
	return string(out), nil
}

func (r *RealSystem) IPSetAdd(setname, entry, comment string) error {
	args := []string{"add", setname, entry}
	if comment != "" {
		args = append(args, "comment", comment)
	}
	if out, err := exec.Command("ipset", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("ipset add %s %s: %w: %s", setname, entry, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *RealSystem) IPSetDel(setname, entry string) error {
	if out, err := exec.Command("ipset", "del", setname, entry).CombinedOutput(); err != nil {
		return fmt.Errorf("ipset del %s %s: %w: %s", setname, entry, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *RealSystem) WGSetPeer(iface, pubkey, allowedIP string) error {
	args := []string{"set", iface, "peer", pubkey, "allowed-ips", allowedIP + "/32"}
	if out, err := exec.Command("wg", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("wg set peer %s: %w: %s", pubkey, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *RealSystem) WGDelPeer(iface, pubkey string) error {
	if out, err := exec.Command("wg", "set", iface, "peer", pubkey, "remove").CombinedOutput(); err != nil {
		return fmt.Errorf("wg del peer %s: %w: %s", pubkey, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *RealSystem) WGGenKey() (string, error) {
	out, err := exec.Command("wg", "genkey").Output()
	if err != nil {
		return "", fmt.Errorf("wg genkey: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *RealSystem) WGPubKey(privkey string) (string, error) {
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = bytes.NewBufferString(privkey + "\n")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("wg pubkey: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
