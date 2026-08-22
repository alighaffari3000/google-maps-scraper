#!/usr/bin/env bash
#
# Builds and starts the stack. Run from this directory, after setup-vps.sh.
set -euo pipefail

cd "$(dirname "$0")"

if [ ! -f .env ]; then
	echo "No .env here. Copy .env.example to .env and fill it in first:" >&2
	echo "  cp .env.example .env && nano .env" >&2
	exit 1
fi

# shellcheck disable=SC1091
set -a && . ./.env && set +a

for var in SITE_ADDRESS BASIC_AUTH_USER BASIC_AUTH_HASH; do
	if [ -z "${!var:-}" ]; then
		echo "$var is empty in .env" >&2
		exit 1
	fi
done

case "$BASIC_AUTH_HASH" in
'$2'*) ;;
*)
	echo "BASIC_AUTH_HASH does not look like a bcrypt hash (should start with \$2)." >&2
	echo "Generate one with:" >&2
	echo "  docker run --rm caddy:2-alpine caddy hash-password --plaintext 'your-password'" >&2
	exit 1
	;;
esac

echo "Building (first run pulls Chromium, so expect 10-20 minutes)..."
docker compose build

echo "Starting..."
docker compose up -d

echo
docker compose ps
echo
echo "Site: https://${SITE_ADDRESS}"
echo "Certificate issuance takes a few seconds on first start."
echo "Watch it with: docker compose logs -f caddy"
