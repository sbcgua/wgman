package main

import (
	"fmt"
	"sort"
	"strings"
)

// AppliedStateDeltas records live operations that completed before a state
// apply returned. It is suitable for dependency-aware rollback.
type AppliedStateDeltas struct {
	PeerAdds    []WGPeerDeltaOp
	IPSetDeltas []IpsetDeltaOp
	PeerRemoves []WGPeerDeltaOp
}

// ApplyIPSetDeltas applies ipset add/delete operations from deltas through sys.
// Operations are applied in order; the first error encountered is returned.
func ApplyIPSetDeltas(deltas []IpsetDeltaOp, sys SystemAdapter) error {
	_, err := ApplyIPSetDeltasTracked(deltas, sys)
	return err
}

// ApplyIPSetDeltasTracked applies deltas and returns the operations that
// completed before any error. Callers can invert those operations for
// best-effort rollback.
func ApplyIPSetDeltasTracked(deltas []IpsetDeltaOp, sys SystemAdapter) ([]IpsetDeltaOp, error) {
	applied := make([]IpsetDeltaOp, 0, len(deltas))
	for _, d := range deltas {
		if d.Add {
			if err := sys.IPSetAdd(d.Set, d.Entry, d.Comment); err != nil {
				return applied, err
			}
		} else {
			if err := sys.IPSetDel(d.Set, d.Entry); err != nil {
				return applied, err
			}
		}
		applied = append(applied, d)
	}
	return applied, nil
}

// InvertIPSetDeltas returns inverse operations in reverse order for rollback.
func InvertIPSetDeltas(deltas []IpsetDeltaOp) []IpsetDeltaOp {
	inverted := make([]IpsetDeltaOp, 0, len(deltas))
	for i := len(deltas) - 1; i >= 0; i-- {
		d := deltas[i]
		d.Add = !d.Add
		inverted = append(inverted, d)
	}
	return inverted
}

func diffExpectedIPSets(cfg *Config, oldAll, oldMatrix, newAll, newMatrix map[string]string) []IpsetDeltaOp {
	var deltas []IpsetDeltaOp
	deltas = append(deltas, diffExpectedIPSet(cfg.Sets.All, oldAll, newAll)...)
	deltas = append(deltas, diffExpectedIPSet(cfg.Sets.Matrix, oldMatrix, newMatrix)...)
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].Set != deltas[j].Set {
			return deltas[i].Set < deltas[j].Set
		}
		if deltas[i].Entry != deltas[j].Entry {
			return deltas[i].Entry < deltas[j].Entry
		}
		return !deltas[i].Add && deltas[j].Add
	})
	return deltas
}

func diffExpectedIPSet(setname string, oldExpected, newExpected map[string]string) []IpsetDeltaOp {
	var deltas []IpsetDeltaOp
	for entry, comment := range newExpected {
		if _, ok := oldExpected[entry]; !ok {
			deltas = append(deltas, IpsetDeltaOp{
				Set:     setname,
				Entry:   entry,
				Comment: comment,
				Add:     true,
			})
		}
	}
	for entry, comment := range oldExpected {
		if _, ok := newExpected[entry]; !ok {
			deltas = append(deltas, IpsetDeltaOp{
				Set:     setname,
				Entry:   entry,
				Comment: comment,
				Add:     false,
			})
		}
	}
	return deltas
}

// ApplyPeerDeltas applies WireGuard peer operations from deltas through sys.
// Operations are applied in order; the first error encountered is returned.
func ApplyPeerDeltas(iface string, deltas []WGPeerDeltaOp, sys SystemAdapter) error {
	_, err := ApplyPeerDeltasTracked(iface, deltas, sys)
	return err
}

// ApplyPeerDeltasTracked applies peer deltas and returns the operations that
// completed before any error. Callers can invert those operations for
// best-effort rollback.
func ApplyPeerDeltasTracked(iface string, deltas []WGPeerDeltaOp, sys SystemAdapter) ([]WGPeerDeltaOp, error) {
	applied := make([]WGPeerDeltaOp, 0, len(deltas))
	for _, d := range deltas {
		switch d.Action {
		case WGPeerAdd:
			if err := sys.WGSetPeer(iface, d.PubKey, d.AllowedIP); err != nil {
				return applied, fmt.Errorf("add WireGuard peer for %q: %w", d.User, err)
			}
		case WGPeerRemove:
			if err := sys.WGDelPeer(iface, d.PubKey); err != nil {
				return applied, fmt.Errorf("remove WireGuard peer for %q: %w", d.User, err)
			}
		default:
			return applied, fmt.Errorf("unknown WireGuard peer delta action %q for %q", d.Action, d.User)
		}
		applied = append(applied, d)
	}
	return applied, nil
}

// InvertPeerDeltas returns inverse peer operations in reverse order for
// rollback.
func InvertPeerDeltas(deltas []WGPeerDeltaOp) []WGPeerDeltaOp {
	inverted := make([]WGPeerDeltaOp, 0, len(deltas))
	for i := len(deltas) - 1; i >= 0; i-- {
		d := deltas[i]
		switch d.Action {
		case WGPeerAdd:
			d.Action = WGPeerRemove
		case WGPeerRemove:
			d.Action = WGPeerAdd
		}
		inverted = append(inverted, d)
	}
	return inverted
}

// ApplyStateDeltas applies WireGuard and ipset operations in a dependency-aware
// order: peer additions, ipset changes, then peer removals.
func ApplyStateDeltas(iface string, ipsetDeltas []IpsetDeltaOp, peerDeltas []WGPeerDeltaOp, sys SystemAdapter) error {
	_, err := ApplyStateDeltasTracked(iface, ipsetDeltas, peerDeltas, sys)
	return err
}

// ApplyStateDeltasTracked applies WireGuard and ipset operations in a
// dependency-aware order and returns the completed operations for rollback.
func ApplyStateDeltasTracked(iface string, ipsetDeltas []IpsetDeltaOp, peerDeltas []WGPeerDeltaOp, sys SystemAdapter) (AppliedStateDeltas, error) {
	var applied AppliedStateDeltas
	var err error

	applied.PeerAdds, err = ApplyPeerDeltasTracked(iface, filterPeerDeltas(peerDeltas, WGPeerAdd), sys)
	if err != nil {
		return applied, err
	}
	applied.IPSetDeltas, err = ApplyIPSetDeltasTracked(ipsetDeltas, sys)
	if err != nil {
		return applied, err
	}
	applied.PeerRemoves, err = ApplyPeerDeltasTracked(iface, filterPeerDeltas(peerDeltas, WGPeerRemove), sys)
	if err != nil {
		return applied, err
	}
	return applied, nil
}

// RollbackStateDeltas rolls back completed state operations in reverse
// dependency order: peer removals, ipset changes, then peer additions.
func RollbackStateDeltas(iface string, applied AppliedStateDeltas, sys SystemAdapter) error {
	var errs []string
	if err := ApplyPeerDeltas(iface, InvertPeerDeltas(applied.PeerRemoves), sys); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ApplyIPSetDeltas(InvertIPSetDeltas(applied.IPSetDeltas), sys); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ApplyPeerDeltas(iface, InvertPeerDeltas(applied.PeerAdds), sys); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func filterPeerDeltas(deltas []WGPeerDeltaOp, action WGPeerDeltaAction) []WGPeerDeltaOp {
	out := make([]WGPeerDeltaOp, 0, len(deltas))
	for _, d := range deltas {
		if d.Action == action {
			out = append(out, d)
		}
	}
	return out
}
