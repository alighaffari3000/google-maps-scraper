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

This adds 2 GB of swap, installs Docker, and closes every port except SSH, 80
and 443. It is safe to re-run.

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

## Expectations

**Google blocks datacenter IP ranges.** A home connection scrapes fine; a VPS
often starts returning empty results within a few jobs. If that happens, the
cause is the IP, not the scraper — add residential proxies in the job form's
Proxies field, or in the `-proxies` flag.

**One job at a time.** Concurrency is pinned to 1 and the container is capped at
1400 MB, because a second concurrent Chromium is what takes a 2 GB host down.
Jobs queue rather than run in parallel, so this costs throughput, not results.
