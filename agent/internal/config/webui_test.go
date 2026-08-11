package config

import (
	"strings"
	"testing"
)

// TestWebUIValidationRejectsAtStartup covers the values that must never reach a client.
//
// A phone composes a URL from these and hands it to an Android intent. A scheme outside
// http/https there would be the operator's config file deciding what a browser opens, and
// a startup failure — where someone is watching — is a far better place to catch that than
// a phone later.
func TestWebUIValidationRejectsAtStartup(t *testing.T) {
	cases := []struct {
		name  string
		block *WebUI
		want  string
	}{
		{"port zero", &WebUI{Port: 0}, "outside 1-65535"},
		{"port negative", &WebUI{Port: -1}, "outside 1-65535"},
		{"port too large", &WebUI{Port: 70000}, "outside 1-65535"},
		{"scheme ftp", &WebUI{Port: 8096, Scheme: "ftp"}, "must be http or https"},
		{"scheme javascript", &WebUI{Port: 8096, Scheme: "javascript"}, "must be http or https"},
		{"scheme file", &WebUI{Port: 8096, Scheme: "file"}, "must be http or https"},
		{"path without leading slash", &WebUI{Port: 8096, Path: "web"}, "must start with /"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.block.validate(0)
			if len(errs) == 0 {
				t.Fatalf("%+v was accepted", tc.block)
			}
			var joined []string
			for _, e := range errs {
				joined = append(joined, e.Error())
			}
			if !strings.Contains(strings.Join(joined, "; "), tc.want) {
				t.Errorf("errors = %v, want one containing %q", joined, tc.want)
			}
		})
	}
}

// TestWebUIAbsenceIsValid: a service with no web interface is not a misconfiguration.
func TestWebUIAbsenceIsValid(t *testing.T) {
	var absent *WebUI
	if errs := absent.validate(0); len(errs) != 0 {
		t.Errorf("an omitted web_ui block produced errors: %v", errs)
	}
	if _, configured := absent.Resolved(); configured {
		t.Error("an omitted block reported itself as configured")
	}
}

func TestWebUIDefaults(t *testing.T) {
	resolved, configured := (&WebUI{Port: 8096}).Resolved()
	if !configured {
		t.Fatal("a configured block reported itself as absent")
	}
	// http, because this is a service on a private network and ADR-0001 puts transport
	// security in the VPN rather than in per-service TLS.
	if resolved.Scheme != "http" {
		t.Errorf("scheme = %q, want the http default", resolved.Scheme)
	}
	if resolved.Path != "/" {
		t.Errorf("path = %q, want the / default", resolved.Path)
	}
	if resolved.Port != 8096 {
		t.Errorf("port = %d", resolved.Port)
	}
}

// TestWebUICarriesNoHost is the security property, asserted structurally.
//
// The agent supplies scheme, port and path and nothing else. If a host or a full URL ever
// appears in this type, a compromised or simply wrong agent could point a client at an
// origin the operator never configured — and the client would open it.
func TestWebUICarriesNoHost(t *testing.T) {
	resolved, _ := (&WebUI{Port: 8096, Scheme: "https", Path: "/web"}).Resolved()

	for _, field := range []string{resolved.Scheme, resolved.Path} {
		if strings.Contains(field, "://") || strings.Contains(field, "..") {
			t.Errorf("field %q looks like an origin; the agent must supply parts only", field)
		}
	}
	if strings.HasPrefix(resolved.Path, "//") {
		// "//evil.example" is a protocol-relative URL: composed onto a scheme it silently
		// replaces the host, which is exactly the redirect this design prevents.
		t.Errorf("path %q is protocol-relative and would override the host", resolved.Path)
	}
}
