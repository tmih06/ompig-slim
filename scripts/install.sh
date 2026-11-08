#!/usr/bin/env bash
# Install a released ompig-slim executable into the user's own ~/.local/bin.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh | bash
#   ... | bash -s -- --version v1.2.3 --dir "$HOME/bin"
#
# The script downloads the prebuilt archive for this operating system and CPU,
# verifies its published SHA-256, and replaces the installed executable with an
# atomic rename. It needs only bash, tar, and curl or wget: no Go toolchain, no
# build, and no sudo. The checksum comes from the same release as the archive,
# so it detects a corrupt or truncated download; it is not an independent
# signature and does not defend against a compromised release.
#
# Environment:
#   OMPIG_SLIM_VERSION       release tag to install (default: latest stable)
#   OMPIG_SLIM_INSTALL_DIR   destination directory (default: $HOME/.local/bin)
#   OMPIG_SLIM_DOWNLOAD_BASE release download root, for mirrors and tests
#                            (default: https://github.com/tmih06/ompig-slim/releases)
#
# Install status is 0 only after the new executable replaced the old one. Every
# failure before that rename leaves an existing installation untouched.
#
# Besides the executable, a successful install records an ownership marker under
# the config root (PIG_HOME, else ~/.pi). `ompig-slim update` and
# `ompig-slim upgrade` only replace a binary that this script installed, so an
# npm, go-install, or container installation keeps its own update path.

# `curl ... | sh` is a common slip; name the requirement instead of failing
# later with an opaque "set: Illegal option -o pipefail". Only POSIX syntax may
# appear above this guard, because it has to run under the wrong shell too.
if [ -z "${BASH_VERSION:-}" ]; then
    printf 'ompig-slim installer: bash is required (try: curl ... | bash)\n' >&2
    exit 1
fi
set -euo pipefail

version="${OMPIG_SLIM_VERSION:-}"
# An explicit --dir or OMPIG_SLIM_INSTALL_DIR must not require HOME, so resolve
# the ~/.local/bin default only after the arguments have been read.
install_dir="${OMPIG_SLIM_INSTALL_DIR:-}"
download_base="${OMPIG_SLIM_DOWNLOAD_BASE:-https://github.com/tmih06/ompig-slim/releases}"

usage() {
    cat <<'EOF'
Install ompig-slim from a published release into ~/.local/bin.

Usage: install.sh [--version TAG] [--dir DIRECTORY]

  --version TAG     install that release tag instead of the latest stable one
  --dir DIRECTORY   install into DIRECTORY (default: $HOME/.local/bin)
  -h, --help        print this help

Environment overrides: OMPIG_SLIM_VERSION, OMPIG_SLIM_INSTALL_DIR,
OMPIG_SLIM_DOWNLOAD_BASE.
EOF
}

# Reject unknown flags instead of silently ignoring a typo that would install
# a different version or location than the caller asked for.
while (($#)); do
    case "$1" in
        --version|--dir)
            if (($# < 2)); then
                printf 'Missing value for %s\n' "$1" >&2
                usage >&2
                exit 2
            fi
            if [[ "$1" == --version ]]; then version="$2"; else install_dir="$2"; fi
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            printf 'Unknown option: %s\n' "$1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ -z "$install_dir" ]]; then
    # Report a missing HOME as a plain message rather than letting ${HOME:?}
    # print this script's path and line number to a piped user.
    if [[ -z "${HOME:-}" ]]; then
        printf 'ompig-slim: HOME must be set, or pass --dir DIRECTORY\n' >&2
        exit 1
    fi
    install_dir="$HOME/.local/bin"
fi

# Release archives exist for exactly these four native builds. Anything else is
# refused before downloading, so a wrong-architecture binary never lands on PATH.
case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *)
        printf 'ompig-slim: unsupported operating system: %s (supported: Linux, macOS)\n' "$(uname -s)" >&2
        exit 1
        ;;
esac
case "$(uname -m)" in
    x86_64|amd64) arch=x64 ;;
    arm64|aarch64) arch=arm64 ;;
    *)
        printf 'ompig-slim: unsupported CPU: %s (supported: x86_64, arm64)\n' "$(uname -m)" >&2
        exit 1
        ;;
esac
platform="$os-$arch"
archive_name="ompig-slim-$platform.tar.gz"
# Mirror GitHub's own release layout: a tagged download lives under
# .../releases/download/<tag>/, while the latest stable release is aliased at
# .../releases/latest/download/. Keeping both shapes here lets
# OMPIG_SLIM_DOWNLOAD_BASE point at a mirror or a test server unchanged.
# "latest" is accepted as a spelling of the default so a configured pin can be
# reset without unsetting the variable.
if [[ -n "$version" && "$version" != latest ]]; then
    archive_url="$download_base/download/$version/$archive_name"
else
    archive_url="$download_base/latest/download/$archive_name"
fi

# tar -z needs a gzip implementation: GNU tar on Linux shells out to it, while
# macOS bsdtar and GNU tar with gzip support link it in. Check for either so a
# missing decompressor is reported instead of surfacing as a tar error. The
# version banner is matched with a shell case pattern to avoid requiring grep.
command -v tar >/dev/null 2>&1 || { printf 'ompig-slim: tar is required\n' >&2; exit 1; }
if ! command -v gzip >/dev/null 2>&1; then
    tar_banner="$(tar --version 2>/dev/null || true)"
    case "$tar_banner" in
        *bsdtar*|*BSDTAR*) ;;
        *) printf 'ompig-slim: gzip is required to unpack %s\n' "$archive_name" >&2; exit 1 ;;
    esac
fi

# fetch <url> <destination>: download with whichever HTTP client is available.
# --fail/--location keep curl from saving an HTML error page as an archive and
# follow the latest-release redirect; wget mirrors that behavior.
fetch() {
    if command -v curl >/dev/null 2>&1; then
        curl --fail --silent --show-error --location --retry 3 --output "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then
        wget --quiet --output-document="$2" "$1"
    else
        printf 'ompig-slim: curl or wget is required to download %s\n' "$1" >&2
        return 1
    fi
}

staged=""
cleanup() {
    if [[ -n "${staged:-}" && -e "$staged" ]]; then rm -f "$staged"; fi
    rm -rf "$work"
}
work="$(mktemp -d "${TMPDIR:-/tmp}/ompig-slim-install.XXXXXX")"
trap cleanup EXIT

archive="$work/$archive_name"
printf 'Downloading %s\n' "$archive_url"
fetch "$archive_url" "$archive"
fetch "$archive_url.sha256" "$archive.sha256"

# The release checksum file is "<digest>  <filename>". Accept only a complete
# lowercase/uppercase hex digest so a mangled or empty file fails loudly.
expected="$(awk 'NR == 1 { print $1 }' "$archive.sha256")"
if [[ "${#expected}" -ne 64 || "$expected" == *[!0-9a-fA-F]* ]]; then
    printf 'ompig-slim: %s.sha256 is not a valid checksum file\n' "$archive_name" >&2
    exit 1
fi
# sha256sum is coreutils (Linux); shasum is the macOS equivalent. Both print
# "<digest>  <filename>", so only the first field is compared.
if command -v sha256sum >/dev/null 2>&1; then
    read -r actual _ < <(sha256sum "$archive")
elif command -v shasum >/dev/null 2>&1; then
    read -r actual _ < <(shasum -a 256 "$archive")
else
    printf 'ompig-slim: sha256sum or shasum is required to verify the download\n' >&2
    exit 1
fi
if [[ "$actual" != "$expected" ]]; then
    printf 'ompig-slim: checksum mismatch for %s\n  expected %s\n  actual   %s\n' \
        "$archive_name" "$expected" "$actual" >&2
    exit 1
fi

# Extract only the executable; the archive also carries README, LICENSE, NOTICE,
# and vendored license texts that belong beside a distribution, not on PATH.
# A release that lacks the executable is rejected before touching the install.
mkdir -p "$work/extract"
if ! tar -xzf "$archive" -C "$work/extract" ompig-slim/ompig-slim 2>/dev/null; then
    printf 'ompig-slim: %s does not contain ompig-slim/ompig-slim\n' "$archive_name" >&2
    exit 1
fi
# Refuse a symlink smuggled under the executable's name: copying it would
# install the link target's contents instead of the released program.
extracted="$work/extract/ompig-slim/ompig-slim"
if [[ ! -f "$extracted" || -L "$extracted" ]]; then
    printf 'ompig-slim: %s has no regular ompig-slim/ompig-slim file\n' "$archive_name" >&2
    exit 1
fi

# Stage inside the destination so the rename is atomic on one filesystem: a
# running instance keeps its old inode, and a failed copy never truncates the
# installed binary. Running the staged copy also keeps the smoke check honest on
# systems that mount /tmp noexec, and any failure leaves the old binary in place.
if ! mkdir -p "$install_dir" || [[ ! -w "$install_dir" ]]; then
    printf 'ompig-slim: cannot write to %s\n' "$install_dir" >&2
    exit 1
fi
target="$install_dir/ompig-slim"
staged="$install_dir/.ompig-slim.new"
cp "$extracted" "$staged"
chmod 0755 "$staged"
if ! output="$("$staged" --version 2>&1)"; then
    printf 'ompig-slim: %s did not run on this machine: %s\n' "$staged" "$output" >&2
    exit 1
fi
mv -f "$staged" "$target"
staged=""

printf 'Installed %s\nVersion: %s\n' "$target" "$output"

# Record that this executable is an archive install, so `ompig-slim update` and
# `ompig-slim upgrade` may replace it and know where the release came from. The
# marker is a hint about provenance, not a security control: it grants no
# privilege the user's own account does not already have. A config root that
# cannot be written only costs the in-place update, so it warns instead of
# failing an installation that already succeeded.
#
# The root follows PiG's own resolution (PIG_HOME, else ~/.pi) and expands a
# leading ~/ the way PiG does, so both programs name the same directory.
config_root="${PIG_HOME:-}"
case "$config_root" in
    '') config_root="${HOME:-}/.pi" ;;
    '~') config_root="${HOME:-}" ;;
    '~/'*) config_root="${HOME:-}/${config_root#'~/'}" ;;
esac
marker="$config_root/ompig-slim-install"
if mkdir -p "$config_root" 2>/dev/null && cat > "$marker" <<EOF 2>/dev/null
kind=archive
executable=$target
download-base=$download_base
installed-version=$output
installer=$archive_url
EOF
then
    chmod 0600 "$marker" 2>/dev/null || true
else
    printf 'warning: could not record the install under %s; `ompig-slim update` will refuse to replace this binary\n' \
        "$config_root" >&2
fi

# A directory that is not on PATH makes the install look broken to the user;
# say exactly what to add instead of leaving them to discover it.
case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf '\n%s is not on your PATH. Add it, for example:\n  export PATH="%s:$PATH"\n' "$install_dir" "$install_dir" ;;
esac
