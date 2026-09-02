#!/usr/bin/env bash
#
# CueSeek agent installer.
#
#   sudo ./install.sh --binary /path/to/cueseekd
#   sudo ./install.sh --uninstall          # keeps config and database
#   sudo ./install.sh --uninstall --purge  # removes them, and the user
#
# Idempotent: safe to re-run to upgrade the binary. It will never overwrite an
# existing /etc/cueseek/config.yaml, and never touches the database.
#
# It does not start the service. A fresh install has no Jellyfin API key, so
# starting it would only produce a failure; the next steps it prints put the
# key in place first.

set -euo pipefail

readonly SERVICE_USER="cueseek"
readonly BIN_DEST="/usr/local/bin/cueseekd"
readonly CONFIG_DIR="/etc/cueseek"
readonly CONFIG_DEST="${CONFIG_DIR}/config.yaml"
readonly STATE_DIR="/var/lib/cueseek"
readonly UNIT_DEST="/etc/systemd/system/cueseekd.service"
readonly POLKIT_DEST="/etc/polkit-1/rules.d/10-cueseek.rules"

# Resolved from this script's own location, so the repository can live anywhere
# and nothing depends on a particular user's home directory.
HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly HERE

BINARY=""
UNINSTALL=0
PURGE=0
# Set when an existing, modified polkit rule was preserved rather than replaced.
# The closing instructions change when that happens, so it must be visible there.
POLKIT_KEPT=0

say()  { printf '  %s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }
die()  { printf '\nERROR: %s\n' "$*" >&2; exit 1; }

usage() {
    sed -n '3,15p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
    exit "${1:-0}"
}

# ---------------------------------------------------------------- arguments

while [ $# -gt 0 ]; do
    case "$1" in
        --binary)    BINARY="${2:-}"; shift 2 ;;
        --uninstall) UNINSTALL=1; shift ;;
        --purge)     PURGE=1; shift ;;
        -h|--help)   usage 0 ;;
        *)           printf 'unknown argument: %s\n' "$1" >&2; usage 1 ;;
    esac
done

[ "$(id -u)" -eq 0 ] || die "must run as root (use sudo)"

# ---------------------------------------------------------------- uninstall

if [ "$UNINSTALL" -eq 1 ]; then
    step "Stopping the service"
    systemctl disable --now cueseekd.service 2>/dev/null || say "not running"

    step "Removing installed files"
    rm -f "$UNIT_DEST" "$POLKIT_DEST" "$BIN_DEST"
    systemctl daemon-reload
    say "removed unit, polkit rule and binary"

    if [ "$PURGE" -eq 1 ]; then
        step "Purging state, configuration and user"
        # Named explicitly rather than rm -rf on a variable: a typo in STATE_DIR
        # should fail, not delete something else.
        rm -rf -- "$STATE_DIR" "$CONFIG_DIR"
        userdel "$SERVICE_USER" 2>/dev/null || true
        say "removed $STATE_DIR, $CONFIG_DIR and the $SERVICE_USER user"
        say "every paired device is now gone and must pair again"
    else
        say "kept $CONFIG_DIR and $STATE_DIR — pass --purge to remove them"
    fi

    printf '\nUninstalled.\n'
    exit 0
fi

# ---------------------------------------------------------------- preflight
#
# Checked before anything is written, so a host that cannot run CueSeek is
# rejected rather than left half-installed.

step "Checking this host"

command -v systemctl >/dev/null 2>&1 || die "systemd not found; CueSeek requires it (ADR-0002)"
say "systemd $(systemctl --version | head -1 | awk '{print $2}')"

# polkit below 0.106 uses .pkla files, which cannot inspect action details. The
# per-unit allowlist would silently degrade to "may restart ANY unit", so this is
# a refusal rather than a warning. M0 identified it as architecture-breaking.
if command -v pkaction >/dev/null 2>&1; then
    polkit_version="$(pkaction --version | awk '{print $NF}')"
    say "polkit ${polkit_version}"
    polkit_major="${polkit_version%%.*}"
    polkit_minor="${polkit_version#*.}"; polkit_minor="${polkit_minor%%.*}"
    [ "$polkit_major" = "$polkit_version" ] && polkit_minor=0
    if [ "$polkit_major" -eq 0 ] && [ "$polkit_minor" -lt 106 ]; then
        die "polkit ${polkit_version} cannot enforce a per-unit allowlist.
     Rules would degrade to granting restart on ANY unit, which is far more
     than 10-cueseek.rules claims. Refusing to install."
    fi
else
    die "pkaction not found; polkit is required (ADR-0002)"
fi

[ -d /etc/polkit-1/rules.d ] || die "/etc/polkit-1/rules.d missing; this polkit does not use JS rules"

if [ -z "$BINARY" ]; then
    for candidate in "$HERE/cueseekd" "$HERE/../agent/dist/cueseekd"; do
        [ -f "$candidate" ] && { BINARY="$candidate"; break; }
    done
fi
[ -n "$BINARY" ] || die "no binary found; pass --binary /path/to/cueseekd"
[ -f "$BINARY" ] || die "binary not found: $BINARY"

# A Windows or macOS cross-build here would install cleanly and then fail to
# execute, which is a confusing way to discover a build mistake.
if command -v file >/dev/null 2>&1; then
    file -b "$BINARY" | grep -q 'ELF.*executable' \
        || die "$BINARY is not a Linux executable — build with GOOS=linux"
fi

# Released tarballs carry a checksum of the binary beside it. Verified when present,
# skipped with a word when not, because building from source is a supported path and has
# nothing to check against.
#
# What this catches is a truncated download or a half-finished extraction. It is NOT a
# signature and proves nothing about origin: anyone who could replace the binary could
# replace this file too. The checksum that carries weight is the one published beside the
# tarball on the release page, out of reach of whoever tampers with its contents.
BINARY_SUMS="$(dirname -- "$BINARY")/cueseekd.sha256"
if [ -f "$BINARY_SUMS" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
        ( cd -- "$(dirname -- "$BINARY")" && sha256sum -c --status cueseekd.sha256 ) \
            || die "$BINARY does not match cueseekd.sha256.
     The download is incomplete or the file has been altered. Fetch it again."
        say "checksum verified"
    else
        say "sha256sum not available; skipping checksum verification"
    fi
else
    say "no cueseekd.sha256 beside the binary; skipping checksum verification"
fi

say "binary $BINARY"

for artifact in cueseekd.service 10-cueseek.rules config.example.yaml; do
    [ -f "$HERE/$artifact" ] || die "missing deployment artifact: $HERE/$artifact"
done

# ---------------------------------------------------------------- user

step "Service user"

if id "$SERVICE_USER" >/dev/null 2>&1; then
    say "$SERVICE_USER already exists"
else
    # System account: no home directory, no login shell. It exists to own a
    # process and a database, and nothing else.
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
    say "created $SERVICE_USER (system, no home, nologin)"
fi

# ---------------------------------------------------------------- files

step "Installing files"

install -m 0755 -o root -g root "$BINARY" "$BIN_DEST"
say "$BIN_DEST"

install -d -m 0755 -o root -g root "$CONFIG_DIR"

if [ -f "$CONFIG_DEST" ]; then
    say "$CONFIG_DEST exists — left untouched"
    install -m 0644 -o root -g root "$HERE/config.example.yaml" \
        "${CONFIG_DIR}/config.example.yaml"
    say "${CONFIG_DIR}/config.example.yaml refreshed for reference"
else
    # 0640 root:cueseek — the file may hold an API key, or name the file that
    # does. The agent reads it as a group member; nobody else can.
    install -m 0640 -o root -g "$SERVICE_USER" "$HERE/config.example.yaml" "$CONFIG_DEST"
    say "$CONFIG_DEST installed from the example — EDIT IT"
fi

# The polkit rule is hand-edited — its allowedUnits list must name the units in
# your config.yaml — so it is protected exactly like config.yaml is.
#
# It was not, until M4.3. `install -m 0644 ... "$POLKIT_DEST"` overwrote it on
# every run, so re-running this script to upgrade the binary silently discarded
# the operator's allowlist and reverted them to the shipped one. The symptom
# arrives later and elsewhere: a restart that worked yesterday is refused by
# polkit today, and nothing connects that to an upgrade.
if [ -f "$POLKIT_DEST" ] && ! cmp -s "$HERE/10-cueseek.rules" "$POLKIT_DEST"; then
    install -m 0644 -o root -g root "$HERE/10-cueseek.rules" "${POLKIT_DEST}.new"
    say "$POLKIT_DEST differs from the shipped rule — left untouched"
    say "  the version from this release is at ${POLKIT_DEST}.new"
    say "  diff them if this upgrade changed what CueSeek needs to be granted"
    POLKIT_KEPT=1
else
    install -m 0644 -o root -g root "$HERE/10-cueseek.rules" "$POLKIT_DEST"
    say "$POLKIT_DEST"
fi

install -m 0644 -o root -g root "$HERE/cueseekd.service" "$UNIT_DEST"
say "$UNIT_DEST"

# systemd creates this from StateDirectory=, but doing it here means the
# directory and its ownership are right even if someone runs the binary by hand.
install -d -m 0750 -o "$SERVICE_USER" -g "$SERVICE_USER" "$STATE_DIR"
say "$STATE_DIR"

step "Reloading systemd"
systemctl daemon-reload
say "done"

# ---------------------------------------------------------------- next steps

cat <<EOF

Installed, and not started yet.

As shipped it manages no services. That is a working install: the agent will
report this machine's own CPU, memory, storage and temperatures with no further
configuration, and show an empty list of services until you add some.

1. Point it at your phone
     sudo nano ${CONFIG_DEST}

     Set bind.address to an address your phone can reach. On Tailscale:
       tailscale ip -4

     Loopback is the default, which is safe and unreachable from anywhere else.

2. Start it
     sudo systemctl enable --now cueseekd.service
     systemctl status cueseekd.service
     journalctl -u cueseekd -f

3. Pair a device
     sudo -u ${SERVICE_USER} ${BIN_DEST} pair -config ${CONFIG_DEST}

     Grants read + service.control. To allow reboot and shutdown from the phone,
     ask for it by name — it is never granted by default:
       ... pair -scopes read,service.control,host.power

At this point you have a working dashboard of the machine itself.

4. Add your services, when you want them
     sudo nano ${CONFIG_DEST}

     Worked examples for every supported type are at the bottom of that file.
     Uncomment one and edit it. Use the EXACT unit name — it often differs from
     what the software calls itself:
       systemctl list-units --type=service | grep -i <name>

5. Then allow those units in the polkit rule
     sudo nano ${POLKIT_DEST}

     Add each unit to allowedUnits. The names there and in the config must
     agree; both are enforced, deliberately (ADR-0002). A service you do not
     list here is still watched — it simply offers no start/stop/restart.

     sudo systemctl restart cueseekd.service

6. Check your work, at any point
     sudo ${BIN_DEST} check -config ${CONFIG_DEST}

     Resolves every configured unit against systemd, compares the config and
     the polkit allowlist in both directions, and probes each service. It
     reports problems before you go looking for them, and changes nothing.

     As root, not as ${SERVICE_USER}: /etc/polkit-1/rules.d is root-only on
     most distributions, so any other user can read the config but not the
     rule, and the allowlist comparison is the point.

If a restart is still refused, this reports which of the three layers said no:
     sudo -u ${SERVICE_USER} ${BIN_DEST} host restart -config ${CONFIG_DEST} <unit>

EOF

if [ "$POLKIT_KEPT" -eq 1 ]; then
    cat <<EOF
NOTE: your existing polkit rule was kept. This release ships a different one at
      ${POLKIT_DEST}.new — compare them before discarding either:

      sudo diff ${POLKIT_DEST} ${POLKIT_DEST}.new

EOF
fi
