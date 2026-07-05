package main

// SystemAdapter abstracts all external command execution and OS interactions.
// Real implementations call wg, ipset, and ip commands.
// Fake implementations are used in tests.
type SystemAdapter interface {
	// IsRoot returns true when the process is running as root (uid 0).
	IsRoot() bool

	// InterfaceSubnet returns the CIDR subnet of the given network interface,
	// e.g. "10.8.0.0/24".
	InterfaceSubnet(iface string) (string, error)

	// WGDump runs "wg show <iface> dump" and returns the raw output.
	WGDump(iface string) (string, error)

	// IPSetList runs "ipset list <setname> -o save" and returns the raw output.
	IPSetList(setname string) (string, error)

	// IPSetCreate creates an ipset with the given type and family inet.
	// withComment enables per-entry comment storage on the set.
	// The creation is idempotent: it is equivalent to "ipset create ... -exist".
	IPSetCreate(setname, setType string, withComment bool) error

	// IPSetAdd runs "ipset add <setname> <entry> [comment <comment>]".
	IPSetAdd(setname, entry, comment string) error

	// IPSetDel runs "ipset del <setname> <entry>".
	IPSetDel(setname, entry string) error

	// WGSetPeer adds or updates a WireGuard peer on the interface.
	WGSetPeer(iface, pubkey, allowedIP string) error

	// WGDelPeer removes a WireGuard peer from the interface.
	WGDelPeer(iface, pubkey string) error

	// WGGenKey generates a new WireGuard private key string.
	WGGenKey() (string, error)

	// WGPubKey derives the public key from a private key string.
	WGPubKey(privkey string) (string, error)
}
