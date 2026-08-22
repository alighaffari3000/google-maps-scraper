#!/usr/bin/env bash
#
# One-shot host preparation for a fresh Debian/Ubuntu VPS: swap, Docker, and a
# firewall that leaves only SSH and the web ports open. Safe to re-run — every
# step checks whether it has already been done.
set -euo pipefail

log() { printf '\n\033[1;34m==>\033[0m %s\n' "$1"; }

if [ "$(id -u)" -ne 0 ]; then
	echo "Run this as root (or with sudo)." >&2
	exit 1
fi

log "Adding swap"
# On a 2 GB host this is not a nicety. Chromium's memory use is spiky, and
# without swap a spike means the OOM killer picks a victim — often sshd.
if swapon --show | grep -q '/swapfile'; then
	echo "swap already active, skipping"
else
	fallocate -l 2G /swapfile
	chmod 600 /swapfile
	mkswap /swapfile
	swapon /swapfile
	grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
fi

log "Lowering swappiness"
# Swap here is an emergency buffer, not a place to page out an idle process.
echo 'vm.swappiness=10' >/etc/sysctl.d/99-swappiness.conf
sysctl -q -p /etc/sysctl.d/99-swappiness.conf

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

log "Configuring the firewall"
if command -v ufw >/dev/null 2>&1 || apt-get install -y -qq ufw; then
	ufw allow OpenSSH >/dev/null
	ufw allow 80/tcp >/dev/null
	ufw allow 443/tcp >/dev/null
	ufw --force enable >/dev/null
	echo "ufw: SSH, 80 and 443 open; everything else closed"
fi

log "Done"
echo "Free memory:"
free -h
echo
echo "Next: cd into the repo's deploy/ directory, create .env, then run ./up.sh"
