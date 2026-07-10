package main

import (
	"fmt"
	"sort"
)

// ApplyDeltas applies ipset add/delete operations from deltas through sys.
// Operations are applied in order; the first error encountered is returned.
func ApplyDeltas(deltas []IpsetDeltaOp, sys SystemAdapter) error {
	_, err := ApplyDeltasTracked(deltas, sys)
	return err
}

// ApplyDeltasTracked applies deltas and returns the operations that completed
// before any error. Callers can invert those operations for best-effort rollback.
func ApplyDeltasTracked(deltas []IpsetDeltaOp, sys SystemAdapter) ([]IpsetDeltaOp, error) {
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

// InvertDeltas returns inverse operations in reverse order for rollback.
func InvertDeltas(deltas []IpsetDeltaOp) []IpsetDeltaOp {
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
	for _, d := range deltas {
		switch {
		case d.Add:
			if err := sys.WGSetPeer(iface, d.PubKey, d.AllowedIP); err != nil {
				return fmt.Errorf("add WireGuard peer for %q: %w", d.User, err)
			}
		case d.Remove:
			if err := sys.WGDelPeer(iface, d.PubKey); err != nil {
				return fmt.Errorf("remove WireGuard peer for %q: %w", d.User, err)
			}
		}
	}
	return nil
}

// ApplyStateDeltas applies WireGuard and ipset operations in a dependency-aware
// order: peer additions, ipset changes, then peer removals.
func ApplyStateDeltas(iface string, ipsetDeltas []IpsetDeltaOp, peerDeltas []WGPeerDeltaOp, sys SystemAdapter) error {
	if err := ApplyPeerDeltas(iface, filterPeerDeltas(peerDeltas, true), sys); err != nil {
		return err
	}
	if err := ApplyDeltas(ipsetDeltas, sys); err != nil {
		return err
	}
	if err := ApplyPeerDeltas(iface, filterPeerDeltas(peerDeltas, false), sys); err != nil {
		return err
	}
	return nil
}

func filterPeerDeltas(deltas []WGPeerDeltaOp, add bool) []WGPeerDeltaOp {
	out := make([]WGPeerDeltaOp, 0, len(deltas))
	for _, d := range deltas {
		if add && d.Add {
			out = append(out, d)
		}
		if !add && d.Remove {
			out = append(out, d)
		}
	}
	return out
}
