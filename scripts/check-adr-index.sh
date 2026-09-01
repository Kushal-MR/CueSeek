#!/usr/bin/env bash
#
# Checks that docs/adr/README.md's index agrees with the records themselves.
#
#   ./scripts/check-adr-index.sh
#
# Exists because of a real defect, not a hypothetical one. During M3.8 an unanchored
# `sed` matched two index rows whose text happened to be identical, so correcting
# ADR-0004's amendment count silently incremented ADR-0006's too. Nothing caught it; it
# was found by reading. The index is the first thing anyone opens and the last thing
# anyone verifies, which is exactly the combination that lets it rot.
#
# Four checks, each mapping to a way the index can lie:
#
#   1. Every record has a row          — a new ADR that nobody indexed
#   2. Every row has a record          — a row pointing at a renamed or deleted file
#   3. Amendment counts match          — the M3.8 defect
#   4. "latest" dates match            — a new amendment with a stale date beside it
#
# It parses rather than renders, so the format it depends on is stated here:
#
#   Index row:  | [0002](0002-host-privilege-dbus-polkit.md) | Title | Accepted · amended ×2 (latest 2026-08-31) |
#   Amendment:  ### Amendment 2 — 2026-08-31: the ceiling widens to reboot and power off
#
# A record with no amendments has no count and no date, and that is not an error.

set -euo pipefail

# Resolved from this script's own location so it runs from anywhere.
HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly ADR_DIR="${HERE}/../docs/adr"
readonly INDEX="${ADR_DIR}/README.md"

[ -f "$INDEX" ] || { printf 'ERROR: no index at %s\n' "$INDEX" >&2; exit 1; }

failures=0
fail() { printf '  FAIL  %s\n' "$*" >&2; failures=$((failures + 1)); }

# ---------------------------------------------------------------- 1 & 2: coverage

# Records on disk. `0000-*.md` through `9999-*.md`; README.md is the index, not a record.
records=()
for path in "$ADR_DIR"/[0-9][0-9][0-9][0-9]-*.md; do
    [ -e "$path" ] || continue
    records+=("$(basename "$path")")
done

[ ${#records[@]} -gt 0 ] || { printf 'ERROR: no ADR files found in %s\n' "$ADR_DIR" >&2; exit 1; }

# Rows in the index, as the filename each links to.
indexed="$(grep -oE '^\| \[[0-9]{4}\]\([^)]+\)' "$INDEX" | sed -E 's/.*\((.*)\)/\1/' || true)"

for record in "${records[@]}"; do
    grep -qxF "$record" <<<"$indexed" \
        || fail "$record exists but has no row in the index"
done

while IFS= read -r linked; do
    [ -n "$linked" ] || continue
    [ -f "${ADR_DIR}/${linked}" ] \
        || fail "the index links to $linked, which does not exist"
done <<<"$indexed"

# ---------------------------------------------------------------- 3 & 4: amendments

for record in "${records[@]}"; do
    file="${ADR_DIR}/${record}"

    # Headings are the source of truth. The header's "Amended:" line and any prose
    # mentioning an amendment are deliberately not counted -- one shape, one meaning.
    actual_count="$(grep -cE '^### Amendment [0-9]+ ' "$file" || true)"
    actual_latest="$(grep -oE '^### Amendment [0-9]+ — [0-9]{4}-[0-9]{2}-[0-9]{2}' "$file" \
        | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | sort | tail -1 || true)"

    row="$(grep -F "($record)" "$INDEX" || true)"
    [ -n "$row" ] || continue   # already reported by check 1

    stated_count="$(grep -oE 'amended ×[0-9]+' <<<"$row" | grep -oE '[0-9]+' || true)"
    stated_latest="$(grep -oE 'latest [0-9]{4}-[0-9]{2}-[0-9]{2}' <<<"$row" \
        | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' || true)"

    if [ "$actual_count" -eq 0 ]; then
        [ -z "$stated_count" ] \
            || fail "$record: index says 'amended ×${stated_count}' but the record has no '### Amendment' heading"
        continue
    fi

    if [ -z "$stated_count" ]; then
        fail "$record: has $actual_count amendment(s) but the index does not say so"
        continue
    fi

    [ "$stated_count" = "$actual_count" ] \
        || fail "$record: index says ×${stated_count}, record has ${actual_count}"

    # Numbering must be dense and start at 1, or a count is not a count.
    expected_last="$(grep -oE '^### Amendment [0-9]+' "$file" \
        | grep -oE '[0-9]+' | sort -n | tail -1)"
    [ "$expected_last" = "$actual_count" ] \
        || fail "$record: highest amendment is numbered ${expected_last} but there are ${actual_count} of them"

    if [ -z "$stated_latest" ]; then
        fail "$record: index states a count but no 'latest' date"
    elif [ "$stated_latest" != "$actual_latest" ]; then
        fail "$record: index says latest ${stated_latest}, newest amendment is ${actual_latest}"
    fi
done

# ---------------------------------------------------------------- result

if [ "$failures" -gt 0 ]; then
    printf '\n%d problem(s). Fix docs/adr/README.md, or the record it disagrees with.\n' \
        "$failures" >&2
    exit 1
fi

printf 'ADR index agrees with all %d records.\n' "${#records[@]}"
