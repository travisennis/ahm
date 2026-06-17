#!/usr/bin/env sh
set -eu

repo="${AHM_REPO:-travisennis/ahm}"
version="${1:-${AHM_VERSION:-latest}}"
install_dir="${BIN_DIR:-${AHM_INSTALL_DIR:-}}"

need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "install.sh: missing required command: $1" >&2
		exit 1
	fi
}

need curl
need grep
need sed
need tar

case "$(uname -s)" in
Darwin) os="darwin" ;;
Linux) os="linux" ;;
*)
	echo "install.sh: unsupported OS: $(uname -s)" >&2
	exit 1
	;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
*)
	echo "install.sh: unsupported architecture: $(uname -m)" >&2
	exit 1
	;;
esac

if [ "$version" = "latest" ]; then
	latest_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")"
	version="${latest_url##*/}"
fi

case "$version" in
v*) tag="$version"; asset_version="${version#v}" ;;
*) tag="v$version"; asset_version="$version" ;;
esac

asset="ahm_${asset_version}_${os}_${arch}.tar.gz"
base_url="https://github.com/$repo/releases/download/$tag"
tmp_dir="$(mktemp -d)"

cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT HUP TERM

archive="$tmp_dir/$asset"
checksums="$tmp_dir/checksums.txt"

curl -fsSL "$base_url/$asset" -o "$archive"
curl -fsSL "$base_url/checksums.txt" -o "$checksums"

expected="$(grep "[[:space:]]$asset\$" "$checksums" | sed 's/[[:space:]].*//')"
if [ -z "$expected" ]; then
	echo "install.sh: checksum entry not found for $asset" >&2
	exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "$archive" | sed 's/ .*//')"
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "$archive" | sed 's/ .*//')"
else
	echo "install.sh: missing sha256sum or shasum for checksum verification" >&2
	exit 1
fi

if [ "$actual" != "$expected" ]; then
	echo "install.sh: checksum mismatch for $asset" >&2
	exit 1
fi

tar -xzf "$archive" -C "$tmp_dir"

if [ -z "$install_dir" ]; then
	if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
		install_dir="/usr/local/bin"
	else
		install_dir="$HOME/.local/bin"
	fi
fi

mkdir -p "$install_dir"
cp "$tmp_dir/ahm" "$install_dir/ahm"
chmod 0755 "$install_dir/ahm"

echo "ahm $tag installed to $install_dir/ahm"
