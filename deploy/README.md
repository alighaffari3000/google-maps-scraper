# Deploying to a VPS

Puts the scraper behind Caddy, which terminates HTTPS and enforces Basic Auth.
The scraper container publishes no ports of its own, so Caddy is the only way in.

Tested target: a 2 GB / 2 vCPU Debian or Ubuntu VPS with ~10 GB free disk
(the Chromium image is large).

## 1. Prepare the host

```
ssh root@YOUR_IP
git clone https://github.com/alighaffari3000/google-maps-scraper.git
cd google-maps-scraper/deploy
./setup-vps.sh
```

This installs Docker and reports on swap, adding a 2 GB swapfile only if the
host has none. It changes nothing else — no firewall rules, no networking, no
touching services already on the box. Safe to re-run.

**Check that ports 80 and 443 are free first.** If the VPS already serves
something, see "Already running a web server" below.

```
ss -tlnp '( sport = :80 or sport = :443 )'
```

## 2. Configure

```
cp .env.example .env
docker run --rm caddy:2-alpine caddy hash-password --plaintext 'pick-a-strong-password'
nano .env
```

Paste the printed hash into `BASIC_AUTH_HASH` and set `SITE_ADDRESS`:

- **No domain?** Use `sslip.io`, which resolves an embedded IP back to itself:
  `193-24-120-105.sslip.io`. That is a real DNS name, so Let's Encrypt will
  issue a real certificate for it.
- **Have a domain?** Point an A record at the VPS and use it here.

## 3. Start

```
./up.sh
```

The first build downloads Chromium and takes 10-20 minutes. Afterwards the site
is at `https://YOUR_SITE_ADDRESS`.

## Day-to-day

| Task | Command |
|---|---|
| Logs | `docker compose logs -f scraper` |
| Restart | `docker compose restart scraper` |
| Update after a `git pull` | `docker compose build && docker compose up -d` |
| Stop | `docker compose down` |
| Memory in use | `docker stats --no-stream` |

Scraped output lives in the `scraper-data` volume, so `down` and rebuilds keep
your jobs. Back it up with:

```
docker run --rm -v deploy_scraper-data:/data -v "$PWD":/backup alpine \
  tar czf /backup/scraper-data.tar.gz -C /data .
```

## Already running a web server

Caddy wants 80 and 443 to obtain and serve its own certificate. If nginx,
Apache, or another Caddy already owns them, do not stop them — publish this
stack on free ports instead and proxy to it from what you already run.

In `.env`:

```
HTTP_PORT=8080
HTTPS_PORT=8443
```

Then point your existing server at `http://127.0.0.1:8080`. Basic Auth still
applies, and TLS is handled by the front server rather than by Caddy, so
`SITE_ADDRESS` should be the hostname that server already serves.

## Expectations

**Google blocks datacenter IP ranges.** A home connection scrapes fine; a VPS
often starts returning empty results within a few jobs. If that happens, the
cause is the IP, not the scraper — add residential proxies in the job form's
Proxies field, or in the `-proxies` flag.

**One job at a time.** Concurrency is pinned to 1 and the container is capped at
1400 MB, because a second concurrent Chromium is what takes a 2 GB host down.
Jobs queue rather than run in parallel, so this costs throughput, not results.
