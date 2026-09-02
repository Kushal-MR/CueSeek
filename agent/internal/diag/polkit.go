package diag

import (
	"errors"
	"fmt"
	"strings"
)

// Reading the polkit rule.
//
// The rule is JavaScript, and this reads it textually rather than evaluating it. That is a
// real limitation and the functions here are shaped around admitting it.
//
// Evaluating it properly would mean embedding a JavaScript engine to answer one question
// about one array — and it still would not be authoritative, because polkit consults every
// rule in the directory and a second file could widen or narrow what this one says. The
// honest position is that this is a **best-effort read of the file CueSeek ships**, and
// that when it cannot be read confidently the answer is "I could not tell", never a guess.
//
// Which direction the uncertainty points matters. Reporting a unit as missing when it is
// granted wastes somebody's evening; reporting it as granted when it is not means `check`
// says everything is fine and the restart still fails. So an array that cannot be located
// is an error rather than an empty result: an empty result would read as "nothing is
// granted", which is a confident claim this cannot support.

// ErrArrayNotFound means the named array literal is not present in a form this can read.
var ErrArrayNotFound = errors.New("array not found in the polkit rule")

// PolkitUnits returns the unit names granted by the rule's allowedUnits array.
func PolkitUnits(raw []byte) ([]string, error) {
	units, err := polkitArray(raw, "allowedUnits")
	if err != nil {
		return nil, fmt.Errorf("reading allowedUnits: %w", err)
	}
	return units, nil
}

// The four logind actions the rule must grant together.
//
// All four or none, as ADR-0002 Amendment 2 records: logind consults the
// `-multiple-sessions` form when another user is logged in, so a rule granting only the
// plain pair works perfectly for whoever tests it alone and fails the first time somebody
// is sitting at the console. That is a failure which appears late, on someone else's
// machine, and looks like a permissions bug — exactly what this command exists to catch
// early.
var requiredPowerActions = []string{
	"org.freedesktop.login1.reboot",
	"org.freedesktop.login1.reboot-multiple-sessions",
	"org.freedesktop.login1.power-off",
	"org.freedesktop.login1.power-off-multiple-sessions",
}

// PolkitPowerActions returns the logind actions granted, and which required ones are absent.
func PolkitPowerActions(raw []byte) (granted []string, missing []string, err error) {
	granted, err = polkitArray(raw, "powerActions")
	if err != nil {
		return nil, nil, fmt.Errorf("reading powerActions: %w", err)
	}

	have := normaliseSet(granted)
	for _, want := range requiredPowerActions {
		if _, ok := have[want]; !ok {
			missing = append(missing, want)
		}
	}
	return granted, missing, nil
}

// polkitArray extracts the string literals from `name = [ ... ]`.
//
// Matching on the assignment rather than on the name alone is load-bearing: `allowedUnits`
// appears twice in the shipped rule — once declared, once as `allowedUnits.indexOf(...)` —
// and a reader that keyed on the name would find the usage first and extract nothing.
func polkitArray(raw []byte, name string) ([]string, error) {
	source := stripJSComments(string(raw))

	open, err := findArrayStart(source, name)
	if err != nil {
		return nil, err
	}

	end := strings.IndexByte(source[open:], ']')
	if end < 0 {
		// A truncated or hand-mangled file. Saying so beats returning the units found
		// before the file ran out, which would be a silently short allowlist.
		return nil, fmt.Errorf("%w: %s is opened but never closed", ErrArrayNotFound, name)
	}

	return stringLiterals(source[open : open+end]), nil
}

// findArrayStart returns the index just past the `[` of `name = [`.
func findArrayStart(source, name string) (int, error) {
	for offset := 0; ; {
		idx := strings.Index(source[offset:], name)
		if idx < 0 {
			return 0, fmt.Errorf("%w: no %s = [ ... ] found", ErrArrayNotFound, name)
		}
		abs := offset + idx
		offset = abs + len(name)

		// `name` must be a whole identifier. Without this, a hypothetical
		// `myAllowedUnits` would satisfy a search for `allowedUnits`.
		if abs > 0 && isIdentByte(source[abs-1]) {
			continue
		}

		rest := strings.TrimLeft(source[offset:], " \t\r\n")
		if !strings.HasPrefix(rest, "=") {
			continue // a usage such as allowedUnits.indexOf(...)
		}
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		if !strings.HasPrefix(rest, "[") {
			continue // assigned something that is not an array literal
		}
		return len(source) - len(rest) + 1, nil
	}
}

func isIdentByte(b byte) bool {
	return b == '_' || b == '$' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// stringLiterals pulls double- and single-quoted strings out of an array body.
func stringLiterals(body string) []string {
	var out []string
	for i := 0; i < len(body); i++ {
		quote := body[i]
		if quote != '"' && quote != '\'' {
			continue
		}
		end := strings.IndexByte(body[i+1:], quote)
		if end < 0 {
			break // unterminated; the caller already treats a malformed file as unreadable
		}
		if literal := strings.TrimSpace(body[i+1 : i+1+end]); literal != "" {
			out = append(out, literal)
		}
		i += end + 1
	}
	return out
}

// stripJSComments blanks out comments while preserving offsets and string contents.
//
// A state machine rather than a regular expression, because the shipped rule is mostly
// comments and several of them contain quotes and unit names in prose. The comment above
// `allowedUnits` names `qbittorrent-nox.service` as an example of what NOT to use, and a
// regex-based reader would happily add it to the allowlist it just parsed.
//
// String awareness matters in the other direction too: a `//` inside a string literal must
// not start a comment. No such string exists in the rule today, and depending on that
// staying true is how this function would quietly break later.
func stripJSComments(src string) string {
	var (
		out         = []byte(src)
		inString    byte // the opening quote, or 0
		inLine      bool
		inBlock     bool
		blankUnless = func(i int) {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	)

	for i := 0; i < len(src); i++ {
		c := src[i]

		switch {
		case inLine:
			blankUnless(i)
			if c == '\n' {
				inLine = false
			}

		case inBlock:
			blankUnless(i)
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				out[i+1] = ' '
				i++
				inBlock = false
			}

		case inString != 0:
			// Escapes are honoured so that a literal ending in a backslash cannot swallow
			// the closing quote and take the rest of the file into the string.
			if c == '\\' && i+1 < len(src) {
				i++
				continue
			}
			if c == inString {
				inString = 0
			}

		case c == '"' || c == '\'':
			inString = c

		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			out[i], out[i+1] = ' ', ' '
			i++
			inLine = true

		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			out[i], out[i+1] = ' ', ' '
			i++
			inBlock = true
		}
	}
	return string(out)
}
