#!/bin/sh
# Install only a verified Anchor executable. Never initializes or edits Agent state.
set -eu

repo='candyflosssss/Anchor'
version=''
install_dir=''
replace=false
work_dir=''
lock_dir=''

fail() { printf 'anchor installer: %s\n' "$*" >&2; exit 1; }
usage() {
    cat <<'HELP'
Usage: sh install.sh [--version VERSION] [--install-dir DIR] [--replace]

Downloads one Linux AMD64 executable and verifies its release SHA256SUMS.
Default version: most recently created published GitHub Release (INCLUDING prereleases).
Default directory: $HOME/.local/bin. No sudo; no shell/Codex/state configuration edits.
  --version VERSION  Pin a stable or alpha/beta/rc version; optional leading v.
  --install-dir DIR  Use an absolute directory (spaces are supported).
  --replace          Explicitly replace an existing regular file. Stop Anchor and
                     back up its state first; this is not a database rollback tool.
  -h, --help         Show this help without downloading or installing anything.
Only Linux x86_64/amd64 is currently published. Other targets fail closed.
HELP
}
while [ "$#" -gt 0 ]; do
    case "$1" in
        --version|--install-dir)
            [ "$#" -ge 2 ] && [ -n "$2" ] || fail "$1 requires a value"
            case "$1" in --version) version=${2#v}; [ -n "$version" ] || fail 'invalid version' ;; --install-dir) install_dir=$2 ;; esac
            shift 2 ;;
        --replace) replace=true; shift ;;
        -h|--help) usage; exit 0 ;;
        *) fail "unknown option: $1 (use --help)" ;;
    esac
done
valid_version() {
    printf '%s\n' "$1" | LC_ALL=C grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(alpha|beta|rc)\.(0|[1-9][0-9]*))?$' || return 1
    # grep is line-oriented; prohibit multi-line input even if one line matches.
    case "$1" in *'
'*) return 1 ;; esac
}
if [ -n "$version" ]; then valid_version "$version" || fail 'invalid version'; fi
case "$(uname -s)" in Linux) ;; *) fail 'unsupported OS; Linux only (Windows: use install.ps1)' ;; esac
case "$(uname -m)" in x86_64|amd64) ;; *) fail 'unsupported architecture; Linux AMD64 only' ;; esac
for tool in curl awk sed grep mktemp sha256sum ln mv; do
    command -v "$tool" >/dev/null 2>&1 || fail "required command not found: $tool"
done
if [ -z "$install_dir" ]; then
    [ -n "${HOME:-}" ] || fail 'HOME is unset; use --install-dir with an absolute directory'
    install_dir=$HOME/.local/bin
fi
case "$install_dir" in /*) ;; *) fail '--install-dir must be absolute' ;; esac
case "$install_dir" in *'
'*|*':'*) fail 'installation directory must not contain a newline or colon' ;; esac

check_destination() {
    [ ! -L "$install_dir/anchor" ] || fail 'refusing a symlink at the destination'
    if [ -e "$install_dir/anchor" ]; then
        [ -f "$install_dir/anchor" ] || fail 'destination is not a regular file'
        [ "$replace" = true ] || fail 'anchor already exists; stop it, back up state, then use --replace'
    fi
}
check_destination
umask 077
mkdir -p "$install_dir"
# Serialize installers; never remove a lock belonging to another process.
mkdir "$install_dir/.anchor-install.lock" 2>/dev/null || fail 'installation is locked; another installer may be running'
lock_dir=$install_dir/.anchor-install.lock
cleanup() {
    if [ -n "$work_dir" ]; then rm -rf "$work_dir"; fi
    if [ -n "$lock_dir" ]; then rmdir "$lock_dir"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP
work_dir=$(mktemp -d "$install_dir/.anchor-install.XXXXXXXX")
fetch() {
    # -q disables .curlrc; downloads and redirects must remain HTTPS.
    curl -q --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --tlsv1.2 --connect-timeout 15 --max-time 180 --retry 2 \
        --header 'Accept: application/vnd.github+json' --output "$2" "$1"
}
if [ -z "$version" ]; then
    fetch "https://api.github.com/repos/$repo/releases?per_page=1" "$work_dir/release.json" || \
        fail 'cannot discover a published release; check network/rate limits, or use --version'
    # Only extract the single tag_name from GitHub's one-release response. Never
    # evaluate response text; the strict version grammar also rejects escaped tags.
    tag_count=$(grep -o '"tag_name"[[:space:]]*:' "$work_dir/release.json" | wc -l)
    [ "$tag_count" -eq 1 ] || fail 'no unambiguous published release; Actions artifacts/drafts cannot be installed here'
    tag=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$work_dir/release.json")
    case "$tag" in v*) version=${tag#v} ;; *) fail 'no supported published release; Actions artifacts/drafts cannot be installed here' ;; esac
    valid_version "$version" || fail 'release has an unsupported version tag; pin a supported --version'
fi
asset=anchor_${version}_linux_amd64
base=https://github.com/$repo/releases/download/v$version
printf 'Installing Anchor %s (Linux AMD64) into %s\n' "$version" "$install_dir"
fetch "$base/SHA256SUMS" "$work_dir/SHA256SUMS" || fail 'release checksum file is unavailable; nothing installed'
expected=$(awk -v name="$asset" '$2 == name || $2 == "*" name { if (NF != 2) exit 1; n++; hash=$1 } END { if (n != 1) exit 1; print hash }' "$work_dir/SHA256SUMS") || \
    fail 'checksum entry is missing, duplicate or malformed'
[ "${#expected}" -eq 64 ] && printf '%s\n' "$expected" | LC_ALL=C grep -Eq '^[0-9a-fA-F]+$' || fail 'invalid SHA256 entry'
fetch "$base/$asset" "$work_dir/anchor" || fail 'release executable is unavailable; nothing installed'
actual=$(sha256sum < "$work_dir/anchor")
actual=${actual%% *}
expected=$(printf '%s' "$expected" | tr 'A-F' 'a-f')
[ "$actual" = "$expected" ] || fail 'SHA256 mismatch; refusing installation (existing program unchanged)'
chmod 755 "$work_dir/anchor"
check_destination
# Staging is on the destination filesystem, so replacement is an atomic rename.
if [ "$replace" = true ]; then
    mv -fT "$work_dir/anchor" "$install_dir/anchor"
else
    # Atomic no-clobber installation; -T never interprets a raced directory as
    # a destination container. Both files are on the same filesystem.
    ln -T "$work_dir/anchor" "$install_dir/anchor"
fi
printf 'Installed: %s/anchor\nNo state, shell profile or Codex configuration was modified.\n' "$install_dir"
case ":${PATH:-}:" in
    *":$install_dir:"*) printf 'Next: anchor version (then anchor init for a new instance).\n' ;;
    *)
        escaped_dir=$(printf '%s' "$install_dir" | sed "s/'/'\\\\''/g")
        printf "Add to PATH in your terminal (the installer cannot change its parent shell):\n  export PATH='%s':\"\$PATH\"\n" "$escaped_dir"
        printf 'Then run: anchor version\n' ;;
esac
