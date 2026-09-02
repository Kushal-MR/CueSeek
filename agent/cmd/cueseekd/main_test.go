package main

import (
	"errors"
	"strings"
	"testing"
)

// The first tests in this package, and they exist because of what their absence cost.
//
// `flag.Parse` stops at the first non-flag argument and reports no error. Nothing checked
// what was left over, so any unrecognised word fell through to the serve path and started
// the daemon — silently discarding every flag after it, including `-config`.
//
// It was found on a real host: `sudo cueseekd check` against a binary that predated the
// `check` subcommand loaded /etc/cueseek/config.yaml, tried to bind the address an agent
// was already listening on, and hung. On a machine where the port had been free it would
// have started a second agent against the real configuration and the real database.
//
// The decision was previously only observable by starting a daemon, which is why it was
// never tested. `classify` exists to make it observable.

func TestClassifyServes(t *testing.T) {
	cases := map[string][]string{
		"no arguments at all": {},
		"a config flag":       {"-config", "/etc/cueseek/config.yaml"},
		"version":             {"-version"},
		"double dash form":    {"--log-level", "debug"},
		"flag's own help":     {"-h"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			sub, rest, err := classify(args)
			if err != nil {
				t.Fatalf("classify(%v) = error %v", args, err)
			}
			if sub != "" {
				t.Errorf("classify(%v) chose subcommand %q, want serve", args, sub)
			}
			if len(rest) != len(args) {
				t.Errorf("serve args = %v, want all of %v", rest, args)
			}
		})
	}
}

func TestClassifySubcommands(t *testing.T) {
	cases := map[string]struct {
		args    []string
		wantSub string
		wantRst []string
	}{
		"check":            {[]string{"check"}, "check", nil},
		"check with flags": {[]string{"check", "-config", "/tmp/c.yaml"}, "check", []string{"-config", "/tmp/c.yaml"}},
		"pair":             {[]string{"pair", "-scopes", "read"}, "pair", []string{"-scopes", "read"}},
		"host":             {[]string{"host", "status", "a.service"}, "host", []string{"status", "a.service"}},
		"help":             {[]string{"help"}, "help", nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sub, rest, err := classify(tc.args)
			if err != nil {
				t.Fatalf("classify(%v) = error %v", tc.args, err)
			}
			if sub != tc.wantSub {
				t.Errorf("subcommand = %q, want %q", sub, tc.wantSub)
			}
			if strings.Join(rest, " ") != strings.Join(tc.wantRst, " ") {
				t.Errorf("rest = %v, want %v", rest, tc.wantRst)
			}
		})
	}
}

// TestClassifyRefusesUnknownWords is the regression test for the defect itself.
//
// Every one of these previously started the agent. The typo cases are the ones that matter
// most: an operator who means `check` and types `chekc` must not launch a daemon, and one
// running a binary too old to have a subcommand must be told that rather than shown a bind
// error from a process they did not think they were starting.
func TestClassifyRefusesUnknownWords(t *testing.T) {
	for name, args := range map[string][]string{
		"a typo of a real subcommand":  {"chekc"},
		"a subcommand from the future": {"doctor"},
		"a word with flags after it":   {"oops", "-config", "/tmp/x.yaml"},
		"a stray path":                 {"/etc/cueseek/config.yaml"},
		"an empty first argument":      {""},
	} {
		t.Run(name, func(t *testing.T) {
			sub, rest, err := classify(args)
			if err == nil {
				t.Fatalf("classify(%v) accepted it as subcommand=%q rest=%v", args, sub, rest)
			}

			var unknown errUnknownSubcommand
			if !errors.As(err, &unknown) {
				t.Fatalf("error is not errUnknownSubcommand: %v", err)
			}
			if unknown.arg != args[0] {
				t.Errorf("error names %q, want the offending argument %q", unknown.arg, args[0])
			}
			// The message must quote the word back. "unknown subcommand" alone leaves the
			// reader guessing which of their arguments was wrong.
			if !strings.Contains(err.Error(), args[0]) {
				t.Errorf("message does not quote the argument: %q", err.Error())
			}
		})
	}
}

// TestServeRefusesPositionalArguments covers the second layer.
//
// classify should stop these before runServe is reached, but the serve path must not be
// enterable with arguments it did not understand no matter how it was called — that is the
// property whose absence discarded a `-config` flag and started an agent against the wrong
// file.
func TestServeRefusesPositionalArguments(t *testing.T) {
	// A path that cannot exist, so that if the guard fails this reports the wrong-argument
	// bug rather than hanging in ListenAndServe against a real configuration.
	err := runServe([]string{"stray", "-config", "/nonexistent/cueseek/config.yaml"})
	if err == nil {
		t.Fatal("runServe accepted a positional argument")
	}
	if !strings.Contains(err.Error(), "stray") {
		t.Errorf("error does not name the unexpected argument: %v", err)
	}
	// It must fail on the argument, not on the config: reporting the config path would
	// mean the flags were parsed and the stray word was still ignored.
	if strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("failed on the config rather than on the stray argument: %v", err)
	}
}

// TestEverySubcommandIsDispatched keeps the map that documents the commands and the switch
// that runs them from drifting apart. A word listed in `help` that main does not handle
// would fall to the default branch and start the agent — the same failure by a different
// route.
func TestEverySubcommandIsDispatched(t *testing.T) {
	dispatched := map[string]bool{
		"check": true, "pair": true, "host": true, "help": true,
	}
	for name := range subcommands {
		if !dispatched[name] {
			t.Errorf("%q is advertised in usage but main has no case for it", name)
		}
	}
	for name := range dispatched {
		if _, ok := subcommands[name]; !ok {
			t.Errorf("main dispatches %q but usage never mentions it", name)
		}
	}
}
