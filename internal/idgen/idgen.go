// Package idgen produces lexicographically-sortable IDs that fit SK's
// varchar(64) primary keys. We use ULID rather than SK's snowflake longs
// because:
//   - ULIDs are 26-char ASCII, drop straight into varchar(64) and JSON,
//   - they're sortable by creation time (close enough to monotonic for
//     paginated listing without an extra ORDER BY column),
//   - no machine/sequence config needed.
//
// SK's existing snowflake-style numeric IDs continue to work — the
// schema column is varchar(64), so ULIDs and the legacy numeric strings
// coexist in the same tables.
package idgen

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

var (
	mu      sync.Mutex
	entropy = ulid.Monotonic(rand.Reader, 0)
)

// New returns a fresh ULID as a 26-char ASCII string.
func New() string {
	mu.Lock()
	defer mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
