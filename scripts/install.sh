#!/bin/sh
# gpu-tools one-line installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/sunerpy/gpu-tools/main/scripts/install.sh | sh
#
# Overrides:
#   GPU_TOOLS_INSTALL_DIR=/path/to/bin  custom install directory
#   GPU_TOOLS_VERSION=v1.2.3            pin a release tag/version
#   sh scripts/install.sh --version v1.2.3 --dir "$HOME/.local/bin"
set -eu

REPO="sunerpy/gpu-tools"
BIN="gpu-tools"
CHECKSUM_FILE="checksums.txt"

requested_version="${GPU_TOOLS_VERSION:-}"
requested_dir="${GPU_TOOLS_INSTALL_DIR:-}"
dry_run=0

err() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

info() {
	printf '%s\n' "$1" >&2
}

usage() {
	cat <<'EOF'
Install gpu-tools from GitHub Releases.

Usage:
  install.sh [--version vX.Y.Z] [--dir DIR] [--dry-run]

Options:
  --version VERSION  Install a specific release tag/version, for example v1.2.3.
                     Defaults to the latest GitHub release.
  --dir DIR          Install directory. Defaults to /usr/local/bin when writable,
                     otherwise falls back to $HOME/.local/bin.
  --dry-run          Print resolved asset URLs and exit without downloading.
  -h, --help         Show this help.

Environment:
  GPU_TOOLS_VERSION      Same as --version.
  GPU_TOOLS_INSTALL_DIR  Same as --dir.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		[ "$#" -ge 2 ] || err "--version requires a value"
		requested_version=$2
		shift 2
		;;
	--dir)
		[ "$#" -ge 2 ] || err "--dir requires a directory"
		requested_dir=$2
		shift 2
		;;
	--dry-run)
		dry_run=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		err "unknown argument: $1"
		;;
	esac
done

if command -v curl >/dev/null 2>&1; then
	download() { curl -fsSL "$1" -o "$2"; }
	fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
	download() { wget -qO "$2" "$1"; }
	fetch() { wget -qO - "$1"; }
else
	err "need curl or wget to download GitHub release assets"
fi

command -v tar >/dev/null 2>&1 || err "need tar to extract release archives"

if command -v sha256sum >/dev/null 2>&1; then
	calc_sha256() { sha256sum "$1" | sed 's/[[:space:]].*$//'; }
elif command -v shasum >/dev/null 2>&1; then
	calc_sha256() { shasum -a 256 "$1" | sed 's/[[:space:]].*$//'; }
else
	err "need sha256sum or shasum to verify checksums"
fi

os_name=$(uname -s)
case "$os_name" in
Linux) os="linux" ;;
Darwin) os="darwin" ;;
*) err "unsupported OS: $os_name (supported: linux, darwin)" ;;
esac

machine=$(uname -m)
case "$machine" in
x86_64 | amd64) arch="amd64" ;;
aarch64 | arm64) arch="arm64" ;;
*) err "unsupported architecture: $machine (supported: amd64, arm64)" ;;
esac

if [ "$requested_version" = "" ] || [ "$requested_version" = "latest" ]; then
	info "Resolving latest release for ${REPO}..."
	api_url="https://api.github.com/repos/${REPO}/releases/latest"
	tag=$(fetch "$api_url" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
	[ "${tag:-}" != "" ] || err "could not resolve latest release tag from ${api_url}"
	release_tag=$tag
else
	case "$requested_version" in
	v*) release_tag=$requested_version ;;
	*) release_tag="v${requested_version}" ;;
	esac
fi

version=$(printf '%s' "$release_tag" | sed 's/^v//')
[ "$version" != "" ] || err "empty release version"

# Must match .goreleaser.yaml archives.name_template:
#   {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
asset_base="${BIN}_${version}_${os}_${arch}"
asset="${asset_base}.tar.gz"
release_base_url="https://github.com/${REPO}/releases/download/${release_tag}"
asset_url="${release_base_url}/${asset}"
checksum_url="${release_base_url}/${CHECKSUM_FILE}"

if [ "$requested_dir" != "" ]; then
	install_dir=$requested_dir
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
	install_dir=/usr/local/bin
else
	install_dir="${HOME}/.local/bin"
fi

info "Installing ${BIN} ${release_tag} (${os}/${arch})"
info "  asset:     ${asset}"
info "  from:      ${asset_url}"
info "  checksum:  ${checksum_url}"
info "  to:        ${install_dir}/${BIN}"

if [ "$dry_run" -eq 1 ]; then
	info "dry-run: no download or install performed"
	exit 0
fi

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t gpu-tools)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

archive_path="${tmp}/${asset}"
checksums_path="${tmp}/${CHECKSUM_FILE}"
extract_dir="${tmp}/extract"
mkdir -p "$extract_dir"

info "Downloading release archive..."
download "$asset_url" "$archive_path" || err "download failed: ${asset_url}"

info "Downloading checksums..."
download "$checksum_url" "$checksums_path" || err "download failed: ${checksum_url}"

checksum_line=$(grep -F "  ${asset}" "$checksums_path" | head -n 1 || true)
[ "$checksum_line" != "" ] || err "${CHECKSUM_FILE} does not contain an entry for ${asset}"
expected_sha=$(printf '%s\n' "$checksum_line" | sed 's/[[:space:]].*$//' | tr '[:upper:]' '[:lower:]')
actual_sha=$(calc_sha256 "$archive_path" | tr '[:upper:]' '[:lower:]')

if [ "$expected_sha" != "$actual_sha" ]; then
	err "checksum mismatch for ${asset}: expected ${expected_sha}, got ${actual_sha}"
fi
info "Checksum verified."

tar -xzf "$archive_path" -C "$extract_dir" || err "failed to extract ${asset}"
[ -f "${extract_dir}/${BIN}" ] || err "archive ${asset} did not contain ${BIN}"

mkdir -p "$install_dir" || err "failed to create install directory: ${install_dir}"
cp "${extract_dir}/${BIN}" "${install_dir}/${BIN}" || err "failed to install ${BIN} to ${install_dir}"
chmod 755 "${install_dir}/${BIN}" || err "failed to mark ${install_dir}/${BIN} executable"

info "installed to ${install_dir}/${BIN}"
case ":${PATH}:" in
*":${install_dir}:"*) ;;
*) info "NOTE: ${install_dir} is not on PATH. Add: export PATH=\"${install_dir}:\$PATH\"" ;;
esac
