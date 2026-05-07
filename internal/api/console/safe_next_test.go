package console

import (
	"strings"
	"testing"
)

// TestIsSafeNext locks down the open-redirect surface of the SK
// bridge. Anything that a browser parses as a host instead of a path
// must come back false. Several of these are paranoid (modern
// browsers no longer honour `\` as `/`, etc.) but the cost of
// rejecting them is zero — there's no legitimate "next" URL that
// starts with whitespace or a backslash.
func TestIsSafeNext(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/", true},
		{"/admin", true},
		{"/admin/poster", true},
		{"/?projectId=abc", true},

		{"", false},
		{"//evil.com", false},
		{`/\evil.com`, false},
		{"/\tevil.com", false},
		{"/ evil.com", false},
		{"/\nevil.com", false},
		{"/\revil.com", false},
		{"https://evil.com", false},
		{"http://evil.com", false},
		{"javascript:alert(1)", false},
		{"\\\\evil.com", false},
		{`%2F%2Fevil.com`, false}, // not pre-decoded; c.Query handles decoding upstream

		// Path that contains "//" past the first byte is fine — the
		// browser resolves /a//b against our origin as a same-host
		// path. We only block leading "//".
		{"/a//b", true},
	}
	for _, tc := range cases {
		if got := isSafeNext(tc.in); got != tc.want {
			t.Errorf("isSafeNext(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestIsSafeNextEdge probes a few more peculiar bytes that could
// theoretically be browser-coerced. None of these are currently a
// known exploit; the test is a tripwire so a future "permissiveness
// cleanup" can't accidentally re-open a class.
func TestIsSafeNextEdge(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		// Multiple separators after the first path char are allowed
		// — the browser just resolves /a//b on our origin.
		{"/foo//bar", true},

		// Form feed / vertical tab — added to the rejected set after
		// Codex flagged that any whitespace after the leading slash
		// is browser-coercion territory.
		{"/\fevil.com", false},
		{"/\vevil.com", false},

		// Embedded \r\n later in the path: now also rejected because
		// url.Parse refuses to round-trip a path containing literal
		// CR/LF (Go 1.21+ rejects them). The defence in depth is
		// fine — no legitimate "next" path embeds CRLF.
		{"/x\r\n\r\nGET / HTTP/1.0", false},

		// Empty after slash — bizarre but harmless.
		{"/", true},

		// Very long path — must not panic.
		{"/" + strings.Repeat("a", 4096), true},
	}
	for _, tc := range cases {
		if got := isSafeNext(tc.in); got != tc.want {
			t.Errorf("isSafeNext(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
