//go:build linux

package metrics

import "golang.org/x/sys/unix"

// Supported reports whether this build can read host metrics at all.
//
// Asked once, at startup, rather than on every collection: a platform does not stop being
// Linux halfway through a session, and an unsupported platform should mean the agent never
// starts the ticker rather than that it emits an all-absent payload ten times a minute.
// Those are different claims — "we cannot look here" versus "we looked and found nothing" —
// and the second one would be untrue.
const Supported = true

// diskUsage measures one filesystem.
//
// Bavail rather than Bfree, deliberately. Bfree includes the reserve — five percent by
// default on ext4 — that only root may spend, so reporting it would offer the operator
// space they cannot use and would make a genuinely full disk look like it still had room.
func diskUsage(path string) (total, free int64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	size := int64(stat.Bsize)
	return int64(stat.Blocks) * size, int64(stat.Bavail) * size, nil
}
