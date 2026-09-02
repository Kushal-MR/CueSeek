// Package diag holds the checks behind `cueseekd check`.
//
// It exists so those checks are testable. The subcommand itself is orchestration —
// load a config, ask some questions, print the answers — and orchestration in `package
// main` is code nothing exercises. The parts with judgement in them live here instead:
// reading an allowlist out of a JavaScript file, deciding what disagreement means, and
// classifying how much a finding matters.
//
// Nothing here performs I/O. Callers read the files and enumerate the interfaces; these
// functions are given bytes and return findings. That is what makes the interesting cases
// — a truncated rule file, an empty allowlist, a unit granted but never configured —
// reachable from a test rather than only from a broken machine.
package diag

import (
	"fmt"
	"sort"
	"strings"
)

// Severity is how much a finding matters.
//
// Three levels, and the middle one carries most of the value. A great deal of what `check`
// reports is neither correct nor broken: a service that is deliberately stopped, a tailnet
// address that has not been assigned yet, a rule granting a unit the config no longer
// names. Collapsing those into "error" would train the operator to ignore the output,
// which is the only failure mode a diagnostic tool really has.
type Severity string

const (
	// SeverityOK means checked and correct. Reported rather than stayed silent about,
	// because "I looked at this and it is fine" is the answer to most of the questions
	// somebody runs this command to ask.
	SeverityOK Severity = "ok"

	// SeverityWarn means working, but worth knowing. Nothing is broken now; something
	// here will surprise somebody later.
	SeverityWarn Severity = "warn"

	// SeverityFail means this will not work. Something the operator intended to happen
	// cannot happen in the current configuration.
	SeverityFail Severity = "fail"
)

// Finding is one answer.
//
// Fix is the field that makes this worth building. `describeHostError` established the
// pattern in M1: the value of a diagnostic is not that it names the problem but that it
// names the next command to run. A finding with no Fix had better be an OK.
type Finding struct {
	Severity Severity

	// Subject is what was examined, e.g. "unit jellyfin.service" or "bind address".
	Subject string

	// Detail is what was found, in a sentence.
	Detail string

	// Fix is what to do about it. Empty for SeverityOK.
	Fix string
}

// Report collects findings in the order they were made.
type Report struct {
	Findings []Finding
}

// OK records a check that passed.
func (r *Report) OK(subject, detail string) {
	r.Findings = append(r.Findings, Finding{
		Severity: SeverityOK, Subject: subject, Detail: detail,
	})
}

// Warn records something working but worth knowing.
func (r *Report) Warn(subject, detail, fix string) {
	r.Findings = append(r.Findings, Finding{
		Severity: SeverityWarn, Subject: subject, Detail: detail, Fix: fix,
	})
}

// Fail records something that will not work.
func (r *Report) Fail(subject, detail, fix string) {
	r.Findings = append(r.Findings, Finding{
		Severity: SeverityFail, Subject: subject, Detail: detail, Fix: fix,
	})
}

// Count returns how many findings carry the given severity.
func (r Report) Count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// HasFailures reports whether anything will not work.
//
// This is what the process exit status is derived from. Warnings deliberately do not
// affect it: a deliberately stopped service must not turn a scripted check red, or nobody
// will script it.
func (r Report) HasFailures() bool { return r.Count(SeverityFail) > 0 }

// UnitComparison is the result of holding the two allowlists side by side.
//
// ADR-0002 requires the same unit names in the agent's configuration and in the polkit
// rule, deliberately not generated from each other, so that two independent things must
// agree before a unit can be touched. Nothing has ever checked that they do, and the
// symptom when they disagree is an authorisation failure that reads like a broken install.
//
// The two directions are genuinely different problems, which is why they are separate
// fields rather than one "mismatch" list:
//
//   - MissingFromRule is a service the operator configured and cannot control. polkit will
//     refuse it. This is a failure.
//   - MissingFromConfig is a unit polkit would allow that the agent will never ask about.
//     Nothing breaks; the machine is simply granting more than it needs to, which is worth
//     saying out loud about a file whose entire job is to be the ceiling.
type UnitComparison struct {
	MissingFromRule   []string
	MissingFromConfig []string
}

// CompareUnits holds the configured units against the granted ones.
//
// Both inputs are normalised and de-duplicated, so a rule listing a unit twice or a config
// with inconsistent spacing does not produce a phantom disagreement.
func CompareUnits(configured, granted []string) UnitComparison {
	inConfig := normaliseSet(configured)
	inRule := normaliseSet(granted)

	var cmp UnitComparison
	for unit := range inConfig {
		if _, ok := inRule[unit]; !ok {
			cmp.MissingFromRule = append(cmp.MissingFromRule, unit)
		}
	}
	for unit := range inRule {
		if _, ok := inConfig[unit]; !ok {
			cmp.MissingFromConfig = append(cmp.MissingFromConfig, unit)
		}
	}

	// Sorted so identical inputs produce identical output. A diagnostic whose text
	// reorders between runs is one nobody can diff.
	sort.Strings(cmp.MissingFromRule)
	sort.Strings(cmp.MissingFromConfig)
	return cmp
}

// Agrees reports whether the two allowlists say the same thing.
func (c UnitComparison) Agrees() bool {
	return len(c.MissingFromRule) == 0 && len(c.MissingFromConfig) == 0
}

func normaliseSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			set[v] = struct{}{}
		}
	}
	return set
}

// Quote renders a list for a message, e.g. `"a.service" and "b.service"`.
//
// Small, and here rather than in the caller because every finding that names a set of
// units should name them the same way.
func Quote(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("%q", values[0])
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}
