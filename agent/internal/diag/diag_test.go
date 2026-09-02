package diag

import (
	"strings"
	"testing"
)

// TestCompareUnitsSeparatesTheTwoDirections.
//
// The two disagreements are different problems with different fixes, which is why they are
// separate fields rather than one "mismatch" list. A unit configured but not granted means
// polkit refuses every restart of it. A unit granted but not configured breaks nothing — it
// simply means the machine grants more than the agent will ever ask for, which is worth
// saying out loud about a file whose whole job is to be the ceiling.
func TestCompareUnitsSeparatesTheTwoDirections(t *testing.T) {
	cmp := CompareUnits(
		[]string{"jellyfin.service", "plexmediaserver.service"},
		[]string{"jellyfin.service", "sonarr.service"},
	)

	if strings.Join(cmp.MissingFromRule, ",") != "plexmediaserver.service" {
		t.Errorf("MissingFromRule = %v", cmp.MissingFromRule)
	}
	if strings.Join(cmp.MissingFromConfig, ",") != "sonarr.service" {
		t.Errorf("MissingFromConfig = %v", cmp.MissingFromConfig)
	}
	if cmp.Agrees() {
		t.Error("Agrees() is true for two lists that disagree in both directions")
	}
}

func TestCompareUnitsAgreement(t *testing.T) {
	cases := map[string]struct {
		configured, granted []string
		agrees              bool
	}{
		"identical":             {[]string{"a.service"}, []string{"a.service"}, true},
		"both empty":            {nil, nil, true},
		"order differs":         {[]string{"a.service", "b.service"}, []string{"b.service", "a.service"}, true},
		"duplicates in rule":    {[]string{"a.service"}, []string{"a.service", "a.service"}, true},
		"whitespace padding":    {[]string{" a.service "}, []string{"a.service"}, true},
		"blank entries in rule": {[]string{"a.service"}, []string{"a.service", "", "  "}, true},
		"nothing configured":    {nil, []string{"a.service"}, false},
		"nothing granted":       {[]string{"a.service"}, nil, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cmp := CompareUnits(tc.configured, tc.granted)
			if cmp.Agrees() != tc.agrees {
				t.Errorf("Agrees() = %v, want %v (missing from rule %v, from config %v)",
					cmp.Agrees(), tc.agrees, cmp.MissingFromRule, cmp.MissingFromConfig)
			}
		})
	}
}

// TestCompareUnitsIsDeterministic: a diagnostic whose text reorders between runs is one
// nobody can diff, and map iteration in Go is deliberately randomised.
func TestCompareUnitsIsDeterministic(t *testing.T) {
	configured := []string{"c.service", "a.service", "b.service"}

	first := CompareUnits(configured, nil)
	for i := 0; i < 20; i++ {
		again := CompareUnits(configured, nil)
		if strings.Join(again.MissingFromRule, ",") != strings.Join(first.MissingFromRule, ",") {
			t.Fatalf("output reordered between runs: %v then %v",
				first.MissingFromRule, again.MissingFromRule)
		}
	}
	if strings.Join(first.MissingFromRule, ",") != "a.service,b.service,c.service" {
		t.Errorf("not sorted: %v", first.MissingFromRule)
	}
}

// TestExitStatusFollowsFailuresOnly.
//
// Warnings must not affect the exit status. A deliberately stopped service is a warning,
// and if that turned a scripted check red nobody would script it — which would cost more
// than the warning is worth.
func TestExitStatusFollowsFailuresOnly(t *testing.T) {
	var r Report
	r.OK("bind address", "reachable")
	r.Warn("jellyfin", "stopped", "start it from the app")

	if r.HasFailures() {
		t.Error("warnings alone report a failure")
	}

	r.Fail("unit sonarr.service", "not in the polkit rule", "add it to allowedUnits")
	if !r.HasFailures() {
		t.Error("a failure was not reported")
	}

	if got := r.Count(SeverityOK); got != 1 {
		t.Errorf("Count(ok) = %d, want 1", got)
	}
	if got := r.Count(SeverityWarn); got != 1 {
		t.Errorf("Count(warn) = %d, want 1", got)
	}
}

// TestEveryNonOKFindingCarriesAFix.
//
// The value of a diagnostic is not that it names the problem but that it names the next
// command to run — the pattern describeHostError established in M1. This asserts the
// constructors keep that promise structurally rather than by review.
func TestEveryNonOKFindingCarriesAFix(t *testing.T) {
	var r Report
	r.OK("a", "fine")
	r.Warn("b", "odd", "do this")
	r.Fail("c", "broken", "do that")

	for _, f := range r.Findings {
		hasFix := strings.TrimSpace(f.Fix) != ""
		if f.Severity == SeverityOK && hasFix {
			t.Errorf("an OK finding carries a fix: %+v", f)
		}
		if f.Severity != SeverityOK && !hasFix {
			t.Errorf("a %s finding has no fix: %+v", f.Severity, f)
		}
		if strings.TrimSpace(f.Subject) == "" || strings.TrimSpace(f.Detail) == "" {
			t.Errorf("finding is missing subject or detail: %+v", f)
		}
	}
}

func TestQuote(t *testing.T) {
	cases := map[string]struct {
		in   []string
		want string
	}{
		"none":  {nil, ""},
		"one":   {[]string{"a.service"}, `"a.service"`},
		"two":   {[]string{"a.service", "b.service"}, `"a.service" and "b.service"`},
		"three": {[]string{"a", "b", "c"}, `"a", "b" and "c"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Quote(tc.in); got != tc.want {
				t.Errorf("Quote(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
