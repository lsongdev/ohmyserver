#!/bin/sh
set -eu

fail() {
    printf 'OhMyServer: %s\n' "$*" >&2
    exit 1
}

[ "$(uname -s)" = Linux ] || fail 'This installer requires Linux with systemd.'
[ "$(id -u)" -eq 0 ] || fail 'Run this script with sudo or as root.'

case "$(uname -m)" in
    x86_64)  binary=ohmyserver_linux_amd64 ;;
    aarch64) binary=ohmyserver_linux_arm64 ;;
    armv7l|armv8l) binary=ohmyserver_linux_arm_v7 ;;
    riscv64) binary=ohmyserver_linux_riscv64 ;;
    *) fail "No Linux release for $(uname -m)." ;;
esac

for command in curl sha256sum systemctl; do
    command -v "$command" >/dev/null 2>&1 || fail "Required command not found: $command"
done
# This works even when the system is degraded, as long as its manager is reachable.
systemctl show --property=Version --value >/dev/null 2>&1 || fail 'Could not reach the systemd system manager.'

release=https://github.com/lsongdev/ohmyserver/releases/latest/download
unit=ohmyserver.service
work_dir=$(mktemp -d)
binary_tmp=
unit_tmp=
cleanup() {
    rm -rf -- "$work_dir"
    [ -z "$binary_tmp" ] || rm -f -- "$binary_tmp"
    [ -z "$unit_tmp" ] || rm -f -- "$unit_tmp"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

cd "$work_dir"
printf 'Downloading the latest OhMyServer release for %s...\n' "$(uname -m)"
for asset in SHA256SUMS "$binary" "$unit"; do
    curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
        --retry 3 --output "$asset" "$release/$asset"
done
for asset in "$binary" "$unit"; do
    awk -v asset="$asset" '$2 == asset { print $1 "  " asset }' SHA256SUMS |
        sha256sum --check --strict - || fail "Checksum verification failed for $asset."
done

mkdir -p /usr/local/bin
(umask 077; mkdir -p /var/lib/ohmyserver)
binary_tmp=$(mktemp /usr/local/bin/.ohmyserver.XXXXXX)
unit_tmp=$(mktemp /etc/systemd/system/.ohmyserver.service.XXXXXX)
install -m 755 "$binary" "$binary_tmp"
install -m 644 "$unit" "$unit_tmp"
mv -fT -- "$binary_tmp" /usr/local/bin/ohmyserver
mv -fT -- "$unit_tmp" /etc/systemd/system/ohmyserver.service

systemctl daemon-reload
systemctl enable "$unit"
systemctl restart "$unit" || fail "Service failed to start. See journalctl -u $unit."
systemctl is-active --quiet "$unit" || fail "Service failed to start. See journalctl -u $unit."

printf '%s\n' 'OhMyServer is running. The default panel address is http://127.0.0.1:8080.' \
    'Configure the service in /var/lib/ohmyserver/config.yaml.'
