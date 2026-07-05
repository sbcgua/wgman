package main

// Config holds the contents of config.yaml.
type Config struct {
	Interface string     `yaml:"interface"`
	Sets      ConfigSets `yaml:"sets"`
}

// ConfigSets names the two ipsets managed by wgman.
type ConfigSets struct {
	All    string `yaml:"all"`
	Matrix string `yaml:"matrix"`
}

// DB holds the contents of db.yaml.
type DB struct {
	Users  map[string]UserEntry `yaml:"users"`
	VMs    map[string]string    `yaml:"vms"`
	Access map[string][]string  `yaml:"access"`
}

// UserEntry is one entry in the users section of db.yaml.
type UserEntry struct {
	IP       string `yaml:"ip"`
	Pub      string `yaml:"pub"`
	Comment  string `yaml:"comment,omitempty"`
	Inactive bool   `yaml:"inactive,omitempty"`
}

// IpsetDeltaOp describes one add or delete operation on an ipset.
type IpsetDeltaOp struct {
	Set     string // ipset name
	Entry   string // e.g. "10.8.0.10,192.168.122.100"
	Comment string // optional comment
	Add     bool   // true = add, false = delete
}

// WGPeerDeltaOp describes one WireGuard peer reconciliation operation.
type WGPeerDeltaOp struct {
	User   string // db username
	PubKey string // WireGuard public key
	Remove bool   // true = remove peer
}

// CheckResult is the structured result returned by the internal check routine.
// HardErrors holds issues that make the state unsafe or ambiguous.
// Drift holds detected discrepancies between expected and live ipset state.
// Deltas holds the concrete ipset operations needed to reconcile drift.
// PeerDeltas holds WireGuard peer operations needed to reconcile safe drift.
// WGDump holds the parsed live WireGuard state; nil if not yet reached.
type CheckResult struct {
	HardErrors []string
	Drift      []string
	Deltas     []IpsetDeltaOp
	PeerDeltas []WGPeerDeltaOp
	WGDump     *WGDumpResult
}

// OK returns true when there are no hard errors and no drift.
func (r *CheckResult) OK() bool {
	return len(r.HardErrors) == 0 && len(r.Drift) == 0 && len(r.PeerDeltas) == 0
}

// Clean returns true when there are no hard errors (drift may still be present).
func (r *CheckResult) Clean() bool {
	return len(r.HardErrors) == 0
}
