package bff

import "testing"

// TestSecureFromURL_FollowsConfiguredScheme is task-10's regression test
// (.chief/milestone-2/_plan/_todo.md): the cookie Secure attribute must be
// derived from cfg.AuthAudience's scheme, not hardcoded. This directly
// covers the bug that broke มายด์'s own first login attempt — an
// http://localhost AuthAudience (GETTING-STARTED.md's documented local-dev
// setup) must produce Secure=false, since Safari refuses to store a Secure
// cookie over plain http. A real (always-https) deployment must still get
// Secure=true, unaffected by this change.
func TestSecureFromURL_FollowsConfiguredScheme(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		secure bool
	}{
		{"http scheme is never secure", "http://localhost:8080", false},
		{"http scheme, no port", "http://localhost", false},
		{"https scheme is always secure", "https://app.example.test", true},
		{"https scheme, local-style host", "https://localhost:8443", true},
		// A URL that fails to parse outright fails safe to Secure=true
		// (the stricter attribute) rather than silently downgrading
		// cookies on a malformed config — see secureFromURL's own doc
		// comment. An empty value parses successfully with an empty
		// scheme, so it lands on the same false branch as any other
		// non-"https" scheme (never reachable in practice: both callers
		// refuse to set any cookie while configured(cfg) is false, which
		// an empty AuthAudience always implies).
		{"unparseable URL fails safe to secure", "://not-a-url", true},
		{"empty URL is not https, not secure", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := secureFromURL(tc.url)
			if got != tc.secure {
				t.Errorf("secureFromURL(%q) = %v, want %v", tc.url, got, tc.secure)
			}
		})
	}
}
