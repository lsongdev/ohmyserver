#!/bin/sh

set -eu

oms_service_name="${OHMYSERVER_SERVICE_NAME:-ohmyserver}"
oms_service_user="${OHMYSERVER_USER:-${SUDO_USER:-ohmyserver}}"
oms_install_path="${OHMYSERVER_INSTALL_PATH:-/usr/local/bin/ohmyserver}"
oms_panel_address="${OHMYSERVER_PANEL_ADDRESS:-0.0.0.0:9000}"
oms_repository="${OHMYSERVER_REPOSITORY:-lsongdev/ohmyserver}"
oms_temp_dir=""
oms_access_token=""

cleanup() {
	if [ -n "$oms_temp_dir" ] && [ -d "$oms_temp_dir" ]; then
		rm -rf "$oms_temp_dir"
	fi
}
trap cleanup EXIT INT TERM

if [ "$(id -u)" -ne 0 ]; then
	echo "install.sh must run as root (use sudo)." >&2
	exit 1
fi

if [ "$#" -ne 0 ]; then
	echo "install.sh downloads the binary from GitHub Latest Release; positional arguments are not supported." >&2
	exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
	echo "curl is required." >&2
	exit 1
fi
if ! command -v sha256sum >/dev/null 2>&1; then
	echo "sha256sum is required." >&2
	exit 1
fi
if ! getent passwd "$oms_service_user" >/dev/null 2>&1; then
	useradd --system --create-home --home-dir "/var/lib/$oms_service_name" \
		--shell /usr/sbin/nologin "$oms_service_user"
fi

oms_service_home="$(getent passwd "$oms_service_user" | cut -d: -f6)"
oms_data_dir="$oms_service_home/.ohmyserver"
oms_database_path="$oms_data_dir/ohmyserver.db"

case "$(uname -s)" in
	Linux) oms_release_os="linux" ;;
	*)
		echo "systemd installation is supported only on Linux." >&2
		exit 1
		;;
esac
case "$(uname -m)" in
	x86_64 | amd64) oms_release_arch="amd64" ;;
	aarch64 | arm64) oms_release_arch="arm64" ;;
	*)
		echo "Unsupported architecture: $(uname -m)" >&2
		exit 1
		;;
esac

oms_asset_name="ohmyserver-$oms_release_os-$oms_release_arch"
oms_temp_dir="$(mktemp -d)"
oms_binary_source="$oms_temp_dir/$oms_asset_name"
oms_checksums_source="$oms_temp_dir/checksums.txt"
if [ -n "${GITHUB_TOKEN:-}" ]; then
	oms_release_json="$oms_temp_dir/release.json"
	curl --fail --silent --show-error --location \
		--header "Authorization: Bearer $GITHUB_TOKEN" \
		--header "Accept: application/vnd.github+json" \
		"https://api.github.com/repos/$oms_repository/releases/latest" \
		--output "$oms_release_json"

	download_private_asset() {
		oms_wanted_asset="$1"
		oms_wanted_destination="$2"
		oms_asset_api_url="$(
			awk -v wanted="$oms_wanted_asset" '
			/"url": "https:\/\/api.github.com\/repos\/.*\/releases\/assets\// {
				url = $2
				gsub(/[",]/, "", url)
			}
			/"name":/ {
				name = $2
				gsub(/[",]/, "", name)
				if (name == wanted) {
					print url
					exit
				}
			}
			' "$oms_release_json"
		)"
		if [ -z "$oms_asset_api_url" ]; then
			echo "Latest release does not contain $oms_wanted_asset." >&2
			exit 1
		fi
		curl --fail --silent --show-error --location \
			--header "Authorization: Bearer $GITHUB_TOKEN" \
			--header "Accept: application/octet-stream" \
			"$oms_asset_api_url" --output "$oms_wanted_destination"
	}
	download_private_asset "$oms_asset_name" "$oms_binary_source"
	download_private_asset "checksums.txt" "$oms_checksums_source"
else
	oms_download_url="https://github.com/$oms_repository/releases/latest/download/$oms_asset_name"
	if ! curl --fail --silent --show-error --location "$oms_download_url" --output "$oms_binary_source"; then
		echo "Release download failed. If the repository is private, run with GITHUB_TOKEN set." >&2
		exit 1
	fi
	curl --fail --silent --show-error --location \
		"https://github.com/$oms_repository/releases/latest/download/checksums.txt" \
		--output "$oms_checksums_source"
fi
if [ ! -s "$oms_binary_source" ]; then
	echo "Downloaded release asset is empty: $oms_asset_name" >&2
	exit 1
fi
oms_verification_file="$oms_temp_dir/verification.txt"
awk -v wanted="$oms_asset_name" '
	$2 == wanted || $2 == "*" wanted {
		print $1 "  " wanted
		found = 1
	}
	END {
		if (!found) exit 1
	}
' "$oms_checksums_source" >"$oms_verification_file" || {
	echo "checksums.txt does not contain $oms_asset_name." >&2
	exit 1
}
(cd "$oms_temp_dir" && sha256sum --check "$(basename "$oms_verification_file")")

systemctl stop "$oms_service_name.service" 2>/dev/null || true

install -d -m 0750 -o "$oms_service_user" -g "$oms_service_user" \
	"$oms_data_dir" "$oms_data_dir/apps" "$oms_data_dir/workspace"
if [ -f "$oms_database_path" ]; then
	oms_backup_path="$oms_database_path.backup.$(date -u +%Y%m%dT%H%M%SZ)"
	cp -p "$oms_database_path" "$oms_backup_path"
	chown "$oms_service_user:$oms_service_user" "$oms_backup_path"
	echo "Database backup: $oms_backup_path"
fi

if [ ! -f "$oms_data_dir/config.yaml" ]; then
	oms_access_token="$(
		od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
	)"
	if [ "${#oms_access_token}" -ne 64 ]; then
		echo "Failed to generate panel access token." >&2
		exit 1
	fi
	oms_config_temp="$(mktemp)"
	cat >"$oms_config_temp" <<EOF
listen: "$oms_panel_address"
access_key: "$oms_access_token"
database:
  driver: sqlite
  path: "$oms_database_path"
collection_interval_seconds: 5
logger:
  level: info
  format: text
EOF
	install -m 0600 -o "$oms_service_user" -g "$oms_service_user" \
		"$oms_config_temp" "$oms_data_dir/config.yaml"
	rm -f "$oms_config_temp"
fi

install -m 0755 "$oms_binary_source" "$oms_install_path"

oms_unit_temp="$(mktemp)"
cat >"$oms_unit_temp" <<EOF
[Unit]
Description=OhMyServer management panel and network services
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$oms_service_user
Group=$oms_service_user
WorkingDirectory=$oms_service_home
RuntimeDirectory=$oms_service_name
RuntimeDirectoryMode=0750
ExecStart=$oms_install_path
Restart=on-failure
RestartSec=3
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
UMask=0027

[Install]
WantedBy=multi-user.target
EOF
install -m 0644 "$oms_unit_temp" "/etc/systemd/system/$oms_service_name.service"
rm -f "$oms_unit_temp"

oms_update_service_temp="$(mktemp)"
cat >"$oms_update_service_temp" <<EOF
[Unit]
Description=Update OhMyServer from the latest verified GitHub release
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
Group=root
TimeoutStartSec=1h
ExecStartPre=/usr/bin/rm -f /run/$oms_service_name/update
ExecStart=$oms_install_path update apply
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
UMask=0027
EOF
install -m 0644 "$oms_update_service_temp" "/etc/systemd/system/$oms_service_name-update.service"
rm -f "$oms_update_service_temp"

oms_update_path_temp="$(mktemp)"
cat >"$oms_update_path_temp" <<EOF
[Unit]
Description=Watch for OhMyServer update requests
After=$oms_service_name.service

[Path]
PathExists=/run/$oms_service_name/update
Unit=$oms_service_name-update.service

[Install]
WantedBy=multi-user.target
EOF
install -m 0644 "$oms_update_path_temp" "/etc/systemd/system/$oms_service_name-update.path"
rm -f "$oms_update_path_temp"

sudo -H -u "$oms_service_user" "$oms_install_path" --init-default-web

systemctl daemon-reload
systemctl enable "$oms_service_name.service"
systemctl enable --now "$oms_service_name-update.path"
systemctl restart "$oms_service_name.service"

oms_attempt=0
while [ "$oms_attempt" -lt 20 ]; do
	if curl --fail --silent --output /dev/null "http://127.0.0.1:9000/" &&
		[ "$(curl --fail --silent --header 'Host: localhost' http://127.0.0.1/)" = "OhMyServer Web Server" ]; then
		echo "OhMyServer panel: http://$(hostname -I | awk '{print $1}'):9000/"
		echo "Default Worker: http://$(hostname -I | awk '{print $1}')/"
		if [ -n "$oms_access_token" ]; then
			echo "Panel Access Token: $oms_access_token"
			echo "Save this token now; it is required to sign in."
		else
			echo "Panel Access Token: preserved from $oms_data_dir/config.yaml"
		fi
		exit 0
	fi
	oms_attempt=$((oms_attempt + 1))
	sleep 1
done

systemctl status "$oms_service_name.service" --no-pager || true
journalctl -u "$oms_service_name.service" -n 50 --no-pager || true
echo "OhMyServer did not become ready." >&2
exit 1
