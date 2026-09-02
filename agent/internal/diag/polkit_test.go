package diag

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadsTheShippedRule is the test that matters most in this file.
//
// Everything else here exercises a hand-written fixture, which proves the parser handles
// the shapes its author imagined. This one reads the actual file `install.sh` puts on a
// real machine, so the parser cannot pass while disagreeing with the thing it exists to
// read — and it fails the day somebody reformats the rule in a way this cannot follow,
// which is precisely when a silent wrong answer would be most expensive.
func TestReadsTheShippedRule(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "10-cueseek.rules")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	units, err := PolkitUnits(raw)
	if err != nil {
		t.Fatalf("cannot read allowedUnits from the shipped rule: %v", err)
	}

	// The shipped rule's allowlist matches the shipped example config's worked examples.
	want := map[string]bool{"jellyfin.service": true, "qbittorrent.service": true}
	if len(units) != len(want) {
		t.Fatalf("allowedUnits = %v, want %d entries", units, len(want))
	}
	for _, u := range units {
		if !want[u] {
			t.Errorf("allowedUnits contains unexpected %q", u)
		}
	}

	// The prose above the array names qbittorrent-nox.service as an example of what NOT
	// to use. A reader that did not strip comments would have granted it.
	for _, u := range units {
		if strings.Contains(u, "nox") {
			t.Errorf("a unit named only in a comment reached the allowlist: %q", u)
		}
	}

	granted, missing, err := PolkitPowerActions(raw)
	if err != nil {
		t.Fatalf("cannot read powerActions from the shipped rule: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("the shipped rule is missing power actions %v", missing)
	}
	if len(granted) != 4 {
		t.Errorf("powerActions = %v, want the four logind actions", granted)
	}
}

// TestUsageBeforeDeclarationIsNotMistakenForIt covers the shape of the real file:
// `allowedUnits` appears as `allowedUnits.indexOf(...)` as well as being declared. A
// reader keying on the name alone finds whichever comes first and can extract nothing.
func TestUsageBeforeDeclarationIsNotMistakenForIt(t *testing.T) {
	rule := `
        if (allowedUnits.indexOf(action.lookup("unit")) >= 0) { return; }
        var allowedUnits = ["a.service", "b.service"];
    `
	units, err := PolkitUnits([]byte(rule))
	if err != nil {
		t.Fatalf("PolkitUnits: %v", err)
	}
	if strings.Join(units, ",") != "a.service,b.service" {
		t.Errorf("units = %v", units)
	}
}

func TestPolkitUnitsVariants(t *testing.T) {
	cases := map[string]struct {
		rule string
		want string
	}{
		"multi-line": {
			"var allowedUnits = [\n  \"a.service\",\n  \"b.service\"\n];", "a.service,b.service"},
		"single line": {`var allowedUnits = ["a.service"];`, "a.service"},
		"no spaces around =": {
			`var allowedUnits=["a.service"];`, "a.service"},
		"single quotes":  {`var allowedUnits = ['a.service'];`, "a.service"},
		"empty array":    {`var allowedUnits = [];`, ""},
		"trailing comma": {`var allowedUnits = ["a.service",];`, "a.service"},
		// The whole point of stripping comments: prose in this file routinely names units.
		"unit named in a line comment": {
			"// do not use b.service or \"c.service\"\nvar allowedUnits = [\"a.service\"];",
			"a.service"},
		"unit named in a block comment": {
			"/* not \"c.service\" */ var allowedUnits = [\"a.service\"];", "a.service"},
		"commented-out entry": {
			"var allowedUnits = [\n  \"a.service\",\n  // \"b.service\"\n];", "a.service"},
		"a similarly named variable is not it": {
			`var myAllowedUnits = ["wrong.service"]; var allowedUnits = ["a.service"];`,
			"a.service"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			units, err := PolkitUnits([]byte(tc.rule))
			if err != nil {
				t.Fatalf("PolkitUnits: %v", err)
			}
			if strings.Join(units, ",") != tc.want {
				t.Errorf("units = %v, want %q", units, tc.want)
			}
		})
	}
}

// TestUnreadableRulesAreErrorsNotEmptyResults is the direction of failure that matters.
//
// An empty result reads as "nothing is granted", which is a confident claim this parser
// cannot support from a file it could not understand. Reporting a unit as missing wastes
// somebody's evening; reporting one as granted when it is not means `check` says everything
// is fine and the restart still fails.
func TestUnreadableRulesAreErrorsNotEmptyResults(t *testing.T) {
	for name, rule := range map[string]string{
		"no such array":        `polkit.addRule(function (a, s) { return; });`,
		"never closed":         `var allowedUnits = ["a.service",`,
		"only ever used":       `if (allowedUnits.indexOf(x) >= 0) { return; }`,
		"assigned a non-array": `var allowedUnits = someOtherList;`,
		"entirely commented":   `/* var allowedUnits = ["a.service"]; */`,
		"empty file":           ``,
	} {
		t.Run(name, func(t *testing.T) {
			units, err := PolkitUnits([]byte(rule))
			if err == nil {
				t.Fatalf("returned %v with no error for an unreadable rule", units)
			}
			if !errors.Is(err, ErrArrayNotFound) {
				t.Errorf("error is not ErrArrayNotFound: %v", err)
			}
		})
	}
}

// TestPowerActionsPartiallyGranted covers ADR-0002 Amendment 2's "all four or none".
//
// A rule granting only the plain forms works perfectly for whoever tests it alone and
// fails the first time somebody is logged in at the console — late, on someone else's
// machine, looking like a permissions bug.
func TestPowerActionsPartiallyGranted(t *testing.T) {
	rule := `var powerActions = [
        "org.freedesktop.login1.reboot",
        "org.freedesktop.login1.power-off"
    ];`

	_, missing, err := PolkitPowerActions([]byte(rule))
	if err != nil {
		t.Fatalf("PolkitPowerActions: %v", err)
	}
	if len(missing) != 2 {
		t.Fatalf("missing = %v, want the two -multiple-sessions variants", missing)
	}
	for _, m := range missing {
		if !strings.Contains(m, "multiple-sessions") {
			t.Errorf("unexpected missing action %q", m)
		}
	}
}

func TestStripJSCommentsPreservesStrings(t *testing.T) {
	// A `//` inside a string must not start a comment. No such string exists in the rule
	// today, and depending on that staying true is how this quietly breaks later.
	src := `var a = ["http://example/x"]; // gone`
	got := stripJSComments(src)

	if !strings.Contains(got, "http://example/x") {
		t.Errorf("a URL inside a string literal was treated as a comment: %q", got)
	}
	if strings.Contains(got, "gone") {
		t.Errorf("the line comment survived: %q", got)
	}
	// Offsets are preserved so that indexes computed on the stripped text are valid
	// against the original.
	if len(got) != len(src) {
		t.Errorf("length changed: %d -> %d", len(src), len(got))
	}
}
