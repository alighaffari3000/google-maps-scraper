#!/usr/bin/env bash
#
# Host preparation for a Debian/Ubuntu VPS: swap and Docker, nothing else.
#
# This deliberately touches nothing it was not asked to. It does not configure
# a firewall, stop services, or alter networking — the server may be running
# other things, and ports 80/443 in particular may already be spoken for.
# Safe to re-run: every step checks whether it has already been done.
set -euo pipefail

log() { printf '\n\033[1;34m==>\033[0m %s\n' "$1"; }

if [ "$(id -u)" -ne 0 ]; then
	echo "Run this as root (or with sudo)." >&2
	exit 1
fi

log "Checking swap"
# Chromium's memory use is spiky, so some swap is the difference between a
# slow scrape and the OOM killer picking a victim. Report what is already
# there and only create swap if the host has none at all — an existing setup
# is the operator's, not ours to second-guess.
swap_kb=$(awk '/^SwapTotal:/ {print $2}' /proc/meminfo)
swap_mb=$((swap_kb / 1024))

if [ "$swap_mb" -ge 2048 ]; then
	echo "${swap_mb} MB of swap already active - leaving it alone"
elif [ "$swap_mb" -gt 0 ]; then
	echo "Only ${swap_mb} MB of swap active. 2048 MB or more is recommended,"
	echo "but adding to an existing setup is left to you."
else
	echo "No swap found. Creating a 2 GB swapfile."
	fallocate -l 2G /swapfile
	chmod 600 /swapfile
	mkswap /swapfile
	swapon /swapfile
	grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
fi

log "Installing Docker"
if command -v docker >/dev/null 2>&1; then
	echo "docker already installed, skipping"
else
	apt-get update -qq
	apt-get install -y -qq ca-certificates curl
	install -m 0755 -d /etc/apt/keyrings
	curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc 2>/dev/null ||
		curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
	chmod a+r /etc/apt/keyrings/docker.asc

	. /etc/os-release
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/${ID} ${VERSION_CODENAME} stable" \
		>/etc/apt/sources.list.d/docker.list

	apt-get update -qq
	apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
	systemctl enable --now docker
fi

log "Done"
echo "Free memory:"
free -h
echo
echo "Ports 80 and 443 must be free for Caddy. Check with:"
echo "  ss -tlnp '( sport = :80 or sport = :443 )'"
echo
echo "Next: create .env in this directory, then run ./up.sh"
