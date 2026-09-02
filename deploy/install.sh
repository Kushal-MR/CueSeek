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

install -m 0644 -o root -g root "$HERE/10-cueseek.rules" "$POLKIT_DEST"
say "$POLKIT_DEST"

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

Installed. The service is NOT running yet — it has no credentials.

1. Create a Jellyfin API key
     Jellyfin Dashboard > API Keys > +, then:

     printf '%s' 'YOUR_KEY' | sudo tee ${CONFIG_DIR}/jellyfin.key > /dev/null
     sudo chown root:${SERVICE_USER} ${CONFIG_DIR}/jellyfin.key
     sudo chmod 0640 ${CONFIG_DIR}/jellyfin.key

2. Edit the configuration
     sudo nano ${CONFIG_DEST}

     At minimum confirm 'unit:' matches this host:
       systemctl list-units --type=service | grep -i jellyfin

     To reach the agent from your phone, set bind.address to your VPN address:
       tailscale ip -4

3. Edit the polkit allowlist to match
     sudo nano ${POLKIT_DEST}

     The unit names there and in the config must agree. Both are enforced.

4. Start it
     sudo systemctl enable --now cueseekd.service
     systemctl status cueseekd.service
     journalctl -u cueseekd -f

5. Pair a device
     sudo -u ${SERVICE_USER} ${BIN_DEST} pair -config ${CONFIG_DEST}

If a restart is refused, this reports which layer said no:
     sudo -u ${SERVICE_USER} ${BIN_DEST} host restart -config ${CONFIG_DEST} jellyfin.service

EOF
