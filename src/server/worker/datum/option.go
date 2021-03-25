package datum

import (
	"context"
	"fmt"
	"path"
	"time"
)

// SetOption configures a set.
type SetOption func(*Set)

// WithMetaOutput sets the Client for the meta output.
func WithMetaOutput(c Client) SetOption {
	return func(s *Set) {
		s.metaOutputClient = c
	}
}

// WithPFSOutput sets the Client for the pfs output.
func WithPFSOutput(c Client) SetOption {
	return func(s *Set) {
		s.pfsOutputClient = c
	}
}

// WithStats sets the stats to fill in.
func WithStats(stats *Stats) SetOption {
	return func(s *Set) {
		s.stats = stats
	}
}

// Option configures a datum.
type Option func(*Datum)

// WithRetry sets the number of retries.
func WithRetry(numRetries int) Option {
	return func(d *Datum) {
		d.numRetries = numRetries
	}
}

// WithRecoveryCallback sets the recovery callback.
func WithRecoveryCallback(cb func(context.Context) error) Option {
	return func(d *Datum) {
		d.recoveryCallback = cb
	}
}

// WithTimeout sets the timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(d *Datum) {
		d.timeout = timeout
	}
}

// WithPrefixIndex prefixes the datum directory name (both locally and in PFS) with its index value.
func WithPrefixIndex() Option {
	return func(d *Datum) {
		d.storageRoot = path.Join(d.set.storageRoot, fmt.Sprintf("%016d", d.meta.Index)+"-"+d.ID)
	}
}