#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
temp_root=$(mktemp -d "${TMPDIR:-/tmp}/commando-install-test.XXXXXX")
cleanup() {
  rm -rf "$temp_root"
}
trap cleanup 0 HUP INT TERM

fail() {
  printf 'install test: %s\n' "$*" >&2
  exit 1
}

checksum() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{ print $1 }'
  else
    shasum -a 256 "$1" | awk '{ print $1 }'
  fi
}

create_asset() {
  release_dir=$1
  version=$2
  os_name=$3
  arch_name=$4
  payload_dir=$temp_root/payload-$os_name-$arch_name
  rm -rf "$payload_dir"
  mkdir -p "$payload_dir"

  if [ "$os_name" = Windows ]; then
    binary_name=commando.exe
    archive_name=commando_${version}_${os_name}_${arch_name}.zip
  else
    binary_name=commando
    archive_name=commando_${version}_${os_name}_${arch_name}.tar.gz
  fi

  cat > "$payload_dir/$binary_name" <<EOF
#!/bin/sh
printf '%s\n' '$os_name/$arch_name'
EOF
  chmod 0755 "$payload_dir/$binary_name"
  mkdir -p "$release_dir"

  if [ "$os_name" = Windows ]; then
    command -v zip >/dev/null 2>&1 || fail "zip is required"
    (cd "$payload_dir" && zip -q "$release_dir/$archive_name" "$binary_name")
  else
    tar -czf "$release_dir/$archive_name" -C "$payload_dir" "$binary_name"
  fi

  printf '%s  %s\n' \
    "$(checksum "$release_dir/$archive_name")" \
    "$archive_name" >> "$release_dir/checksums.txt"
}

assert_install() {
  os_name=$1
  arch_name=$2
  expected=$3
  install_dir=$4
  shift 4

  output=$(
    COMMANDO_OS=$os_name \
    COMMANDO_ARCH=$arch_name \
    COMMANDO_RELEASES_URL=file://$temp_root/releases \
    COMMANDO_INSTALL_DIR="$install_dir" \
      sh "$repo_root/install.sh" "$@"
  )

  case "$os_name" in
    Windows) binary_name=commando.exe ;;
    *) binary_name=commando ;;
  esac
  [ -x "$install_dir/$binary_name" ] || fail "$binary_name was not installed"
  actual=$("$install_dir/$binary_name")
  [ "$actual" = "$expected" ] ||
    fail "installed payload is $actual, expected $expected"
  printf '%s\n' "$output" | grep -q "Installed commando to" ||
    fail "success output is missing"
}

latest_dir=$temp_root/releases/latest/download
create_asset "$latest_dir" 1.2.3 Linux x86_64
create_asset "$latest_dir" 1.2.3 Darwin arm64
create_asset "$latest_dir" 1.2.3 Windows arm64

assert_install Linux amd64 Linux/x86_64 "$temp_root/linux bin"
assert_install Darwin aarch64 Darwin/arm64 "$temp_root/darwin-bin"
assert_install Windows arm64 Windows/arm64 "$temp_root/windows-bin"

pinned_dir=$temp_root/releases/download/v1.1.0
create_asset "$pinned_dir" 1.1.0 Linux arm64
assert_install Linux arm64 Linux/arm64 "$temp_root/pinned-bin" --version 1.1.0

bad_dir=$temp_root/releases/download/v9.9.9
create_asset "$bad_dir" 9.9.9 Linux x86_64
printf 'corrupt\n' >> "$bad_dir/commando_9.9.9_Linux_x86_64.tar.gz"
if COMMANDO_OS=Linux \
  COMMANDO_ARCH=x86_64 \
  COMMANDO_RELEASES_URL=file://$temp_root/releases \
  COMMANDO_INSTALL_DIR="$temp_root/bad-bin" \
    sh "$repo_root/install.sh" --version v9.9.9 >"$temp_root/bad.out" 2>&1; then
  fail "checksum mismatch unexpectedly succeeded"
fi
grep -q "checksum verification failed" "$temp_root/bad.out" ||
  fail "checksum failure was not reported"
[ ! -e "$temp_root/bad-bin/commando" ] ||
  fail "a corrupt binary was installed"

if COMMANDO_OS=Plan9 \
  COMMANDO_ARCH=x86_64 \
    sh "$repo_root/install.sh" >"$temp_root/unsupported.out" 2>&1; then
  fail "unsupported platform unexpectedly succeeded"
fi
grep -q "unsupported operating system" "$temp_root/unsupported.out" ||
  fail "unsupported platform was not reported"

printf 'installer tests passed\n'
