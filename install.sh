#!/bin/sh
set -eu

project=commando
releases_url=${COMMANDO_RELEASES_URL:-https://github.com/liliumapps/commando/releases}
version=${COMMANDO_VERSION:-latest}
install_dir=${COMMANDO_INSTALL_DIR:-${HOME:?HOME must be set}/.local/bin}

usage() {
  cat <<'EOF'
Install Commando from a GitHub release.

Usage:
  install.sh [--version VERSION] [--install-dir DIRECTORY]

Environment:
  COMMANDO_VERSION       Release to install (default: latest)
  COMMANDO_INSTALL_DIR   Destination directory (default: ~/.local/bin)
  COMMANDO_RELEASES_URL  Alternate release base URL
EOF
}

fail() {
  printf 'commando installer: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail "--version requires a value"
      version=$2
      shift 2
      ;;
    --version=*)
      version=${1#*=}
      shift
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail "--install-dir requires a value"
      install_dir=$2
      shift 2
      ;;
    --install-dir=*)
      install_dir=${1#*=}
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[ -n "$version" ] || fail "version cannot be empty"
[ -n "$install_dir" ] || fail "install directory cannot be empty"

os_name=${COMMANDO_OS:-$(uname -s)}
case "$os_name" in
  Darwin|darwin)
    release_os=Darwin
    archive_extension=tar.gz
    binary_name=$project
    ;;
  Linux|linux)
    release_os=Linux
    archive_extension=tar.gz
    binary_name=$project
    ;;
  CYGWIN*|MINGW*|MSYS*|Windows|windows)
    release_os=Windows
    archive_extension=zip
    binary_name=$project.exe
    ;;
  *)
    fail "unsupported operating system: $os_name"
    ;;
esac

arch_name=${COMMANDO_ARCH:-$(uname -m)}
case "$arch_name" in
  x86_64|amd64)
    release_arch=x86_64
    ;;
  arm64|aarch64)
    release_arch=arm64
    ;;
  *)
    fail "unsupported architecture: $arch_name"
    ;;
esac

if command -v curl >/dev/null 2>&1; then
  download() {
    curl --fail --silent --show-error --location --output "$2" "$1"
  }
elif command -v wget >/dev/null 2>&1; then
  download() {
    wget --quiet --output-document="$2" "$1"
  }
else
  fail "curl or wget is required"
fi

if [ "$version" = latest ]; then
  release_url=$releases_url/latest/download
else
  case "$version" in
    v*) tag=$version ;;
    *) tag=v$version ;;
  esac
  release_url=$releases_url/download/$tag
fi

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/commando-install.XXXXXX")
cleanup() {
  rm -rf "$temp_dir"
}
trap cleanup 0 HUP INT TERM

checksums_path=$temp_dir/checksums.txt
printf 'Fetching release metadata for %s/%s...\n' "$release_os" "$release_arch"
download "$release_url/checksums.txt" "$checksums_path"

asset_suffix=_${release_os}_${release_arch}.${archive_extension}
checksum_line=$(
  awk -v suffix="$asset_suffix" '
    length($2) >= length(suffix) &&
    substr($2, length($2) - length(suffix) + 1) == suffix {
      print $1, $2
      exit
    }
  ' "$checksums_path"
)
[ -n "$checksum_line" ] ||
  fail "release does not contain an archive for $release_os/$release_arch"

expected_checksum=$(printf '%s\n' "$checksum_line" | awk '{ print $1 }')
archive_name=$(printf '%s\n' "$checksum_line" | awk '{ print $2 }')
case "$archive_name" in
  */*|*".."*)
    fail "invalid archive name in checksum manifest"
    ;;
esac

archive_path=$temp_dir/$archive_name
printf 'Downloading %s...\n' "$archive_name"
download "$release_url/$archive_name" "$archive_path"

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum=$(sha256sum "$archive_path" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual_checksum=$(shasum -a 256 "$archive_path" | awk '{ print $1 }')
elif command -v openssl >/dev/null 2>&1; then
  actual_checksum=$(openssl dgst -sha256 "$archive_path" | awk '{ print $NF }')
else
  fail "sha256sum, shasum, or openssl is required to verify the download"
fi

[ "$actual_checksum" = "$expected_checksum" ] ||
  fail "checksum verification failed for $archive_name"

extract_dir=$temp_dir/extracted
mkdir -p "$extract_dir"
case "$archive_extension" in
  tar.gz)
    tar -xzf "$archive_path" -C "$extract_dir"
    ;;
  zip)
    command -v unzip >/dev/null 2>&1 || fail "unzip is required on Windows"
    unzip -q "$archive_path" -d "$extract_dir"
    ;;
esac

source_binary=$extract_dir/$binary_name
[ -f "$source_binary" ] || fail "$binary_name was not found in $archive_name"

mkdir -p "$install_dir"
target=$install_dir/$binary_name
cp "$source_binary" "$target"
chmod 0755 "$target"

printf 'Installed %s to %s\n' "$project" "$target"
case ":${PATH:-}:" in
  *":$install_dir:"*) ;;
  *)
    printf 'Add %s to PATH before using %s.\n' "$install_dir" "$project"
    ;;
esac

cat <<'EOF'

Add shell integration:
  Zsh:  echo 'eval "$(commando init zsh)"' >> ~/.zshrc
  Bash: echo 'eval "$(commando init bash)"' >> ~/.bashrc
  Fish: echo 'commando init fish | source' >> ~/.config/fish/config.fish
EOF
