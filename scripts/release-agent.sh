#!/usr/bin/env bash
#
# Builds the release tarball for the CueSeek agent.
#
#   ./scripts/release-agent.sh                 # version from `git describe`
#   ./scripts/release-agent.sh -v v0.1.0       # version stated explicitly
#   ./scripts/release-agent.sh -o /tmp/out     # somewhere other than dist/
#
# One script rather than build steps written into the workflow, so that a release built by
# hand and a release built by CI are the same bytes produced the same way. A workflow that
# is the only thing which knows how to build is a workflow nobody can reproduce locally
# when it breaks.
#
# For ordinary development, `cd agent && go build ./...` is still the fast path. This is
# for producing something somebody else will download and run as root.

set -euo pipefail

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT="${HERE}/.."

readonly GOOS_TARGET="linux"
readonly GOARCH_TARGET="amd64"

OUT="${ROOT}/dist"
VERSION=""

say()  { printf '  %s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }
die()  { printf '\nERROR: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        -v|--version) VERSION="${2:-}"; shift 2 ;;
        -o|--out)     OUT="${2:-}"; shift 2 ;;
        -h|--help)    sed -n '3,15p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *)            die "unknown argument: $1" ;;
    esac
done

# ---------------------------------------------------------------- version
#
# The `-X main.version` hook has been in main.go since M1 and nothing ever used it, so
# every binary ever built reported 0.0.0-dev — including the one installed on the
# development host, which is how a `check` subcommand that did not exist yet came to be
# passed to a daemon (M4.5b). A version that is always the same is a version that cannot
# answer "is this the build with the fix in it".

if [ -z "$VERSION" ]; then
    VERSION="$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")"
fi

# `--dirty` is a warning, not a refusal: building from a modified tree is a normal thing to
# do while testing the packaging itself. It must never be published, and the suffix in the
# version string is what makes that visible afterwards rather than at the moment it matters.
case "$VERSION" in
    *-dirty) say "WARNING: the working tree is modified; this build is not reproducible" ;;
esac

readonly NAME="cueseek-agent_${VERSION}_${GOOS_TARGET}_${GOARCH_TARGET}"
readonly STAGE="${OUT}/${NAME}"

step "Building $VERSION for ${GOOS_TARGET}/${GOARCH_TARGET}"

rm -rf "$STAGE"
mkdir -p "$STAGE"

# CGO_ENABLED=0 is the load-bearing one, and it must be explicit rather than inherited.
# Cross-compiling from a machine without a C toolchain disables cgo anyway, so this looks
# redundant when tested locally — but a Linux CI runner has one, would enable cgo by
# default, and would produce a binary dynamically linked against that runner's glibc. It
# would run on the runner, run on the maintainer's Ubuntu, and fail on somebody's Alpine.
#
# modernc.org/sqlite is pure Go precisely so this is available (see agent/go.mod).
#
# -trimpath removes the building machine's directory layout from the binary. It is what
# stops a released artefact carrying the path of whatever laptop produced it, and it is a
# precondition for two people building the same tag and getting the same bytes.
#
# Deliberately NOT -ldflags "-s -w". Stripping the symbol table saves about 4 MB out of 18
# and costs legible stack traces in the journal, on a daemon whose entire diagnostic story
# is that the operator can find out what happened. A one-off download is the wrong place to
# economise.
CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
    go -C "${ROOT}/agent" build \
        -trimpath \
        -ldflags "-X main.version=${VERSION}" \
        -o "${STAGE}/cueseekd" \
        ./cmd/cueseekd

# Set explicitly rather than inherited from whatever `go build` produced.
#
# Windows has no executable bit, so a tarball built on the development machine shipped the
# binary as 0644 and `install.sh` on the far end could not run it. On a Linux runner the
# mode is right by luck, which is what makes it worth pinning: the failure appears only
# when the release is cut somewhere other than CI, and only for the person who downloads
# it. Same class of defect as deploy/install.sh being committed non-executable (M4.2).
chmod 0755 "${STAGE}/cueseekd"

say "cueseekd $(du -h "${STAGE}/cueseekd" | cut -f1)"

# A dynamically linked release binary is the failure this build is shaped to avoid, and it
# fails on somebody else's distribution rather than here. Checked rather than assumed.
if command -v file >/dev/null 2>&1; then
    if ! file -b "${STAGE}/cueseekd" | grep -q 'statically linked'; then
        die "the binary is not statically linked; CGO_ENABLED=0 did not take effect"
    fi
    say "statically linked"
fi

# ---------------------------------------------------------------- payload
#
# The tarball is self-contained on purpose. install.sh already looks for the binary at
# $HERE/cueseekd, so shipping it alongside its own artefacts means a stranger runs two
# commands rather than cloning a repository to obtain four files.

step "Assembling the payload"

for artefact in install.sh cueseekd.service 10-cueseek.rules config.example.yaml; do
    [ -f "${ROOT}/deploy/${artefact}" ] || die "missing deploy/${artefact}"
    install -m 0644 "${ROOT}/deploy/${artefact}" "${STAGE}/${artefact}"
    say "$artefact"
done
chmod 0755 "${STAGE}/install.sh"

# Apache-2.0 section 4(a) requires giving recipients a copy of the License, and 4(d)
# requires carrying the NOTICE. Not optional, and not something to remember by hand at
# publish time.
for legal in LICENSE NOTICE; do
    [ -f "${ROOT}/${legal}" ] || die "missing ${legal}"
    install -m 0644 "${ROOT}/${legal}" "${STAGE}/${legal}"
    say "$legal"
done

# Checksum of the binary, inside the tarball, so install.sh can confirm that what it is
# about to place in /usr/local/bin is what was released — against a truncated download or a
# half-finished extraction. It is NOT a signature and proves nothing about origin: an
# attacker who replaced the binary would replace this too. The checksum that carries weight
# is the one published beside the tarball, out of reach of whoever tampers with its
# contents.
( cd "$STAGE" && sha256sum cueseekd > cueseekd.sha256 )
say "cueseekd.sha256"

# ---------------------------------------------------------------- archive

step "Archiving"

readonly TARBALL="${OUT}/${NAME}.tar.gz"
readonly PLAIN_TAR="${OUT}/${NAME}.tar"

# Modes are stated here rather than read off the filesystem, in two passes.
#
# The first attempt archived whatever the working tree happened to hold, and shipped
# cueseekd as 0644: this project is developed on Windows, where /tmp is an NTFS mount with
# `noacl,posix=0` and the executable bit is *inferred* rather than stored — from a shebang,
# which install.sh has and a Linux ELF does not. `chmod 0755` there is a no-op. On a Linux
# runner the mode would have been right by luck, so the broken tarball would only ever be
# produced by a release cut outside CI, and only noticed by whoever downloaded it.
#
# Naming the modes makes the archive a property of this script rather than of the machine
# that ran it, which is also what lets two people building the same tag get the same bytes.
#
# --sort=name, fixed ownership, and an mtime taken from the commit rather than the clock,
# for the same reason. gzip -n keeps the filename and timestamp out of the gzip header,
# which would otherwise differ on every run.
readonly TAR_COMMON=(--sort=name --owner=0 --group=0 --numeric-owner
    --mtime="@$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || echo 0)")

tar "${TAR_COMMON[@]}" --mode=0755 --no-recursion -cf "$PLAIN_TAR" -C "$OUT" \
    "$NAME" "${NAME}/cueseekd" "${NAME}/install.sh"

tar "${TAR_COMMON[@]}" --mode=0644 -rf "$PLAIN_TAR" -C "$OUT" \
    "${NAME}/cueseekd.service" \
    "${NAME}/10-cueseek.rules" \
    "${NAME}/config.example.yaml" \
    "${NAME}/cueseekd.sha256" \
    "${NAME}/LICENSE" \
    "${NAME}/NOTICE"

gzip -n -f "$PLAIN_TAR"
[ -f "$TARBALL" ] || die "gzip did not produce $TARBALL"

# The last gate, and it inspects the ARCHIVE rather than the working tree — the working
# tree is exactly what cannot be trusted to carry a mode here.
step "Verifying the archive"
archived="$(tar -tvzf "$TARBALL")"
printf '%s\n' "$archived" | grep -qE '^-rwxr-xr-x .*/cueseekd$' \
    || die "cueseekd is not 0755 inside the tarball:
$(printf '%s\n' "$archived" | grep -E '/cueseekd$')"
printf '%s\n' "$archived" | grep -qE '^-rwxr-xr-x .*/install.sh$' \
    || die "install.sh is not 0755 inside the tarball"
say "cueseekd and install.sh are executable"

# Every file the payload promised, still there after two tar passes. A file silently
# dropped by a mistyped path in the second pass would otherwise surface as a missing
# artefact during somebody else's install.
for expect in cueseekd install.sh cueseekd.service 10-cueseek.rules \
              config.example.yaml cueseekd.sha256 LICENSE NOTICE; do
    printf '%s\n' "$archived" | grep -qE "/${expect}\$" \
        || die "$expect is missing from the tarball"
done
say "all 8 files present"

( cd "$OUT" && sha256sum "${NAME}.tar.gz" > SHA256SUMS )

say "$(basename "$TARBALL") $(du -h "$TARBALL" | cut -f1)"
say "SHA256SUMS"

printf '\nBuilt %s\n\n' "$VERSION"
printf '  %s\n' "$TARBALL"
printf '  %s\n\n' "${OUT}/SHA256SUMS"
