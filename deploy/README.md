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

## Using the prebuilt image (recommended)

Building on the VPS means compiling Go and downloading Chromium on the box
that has the least capacity to do it. The `webapp image` workflow builds on
GitHub's runners instead and publishes to GHCR; the VPS then pulls a finished
image in a couple of minutes.

Run the workflow once from the repository's Actions tab, then make the
resulting package public (Packages -> google-maps-scraper -> Package settings
-> Change visibility), so the VPS can pull without logging in.

`SCRAPER_IMAGE` in `.env` selects it. To deploy, and to update later:

```
docker compose -f docker-compose.yaml -f docker-compose.behind-proxy.yaml pull
docker compose -f docker-compose.yaml -f docker-compose.behind-proxy.yaml up -d
```

Unset `SCRAPER_IMAGE` to go back to building from source.

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

Caddy needs 80 and 443 to obtain and serve its own certificate. If nginx or
Apache already owns them, do not stop it — run the scraper on the loopback
interface and proxy to it from the server you already have. Then that server
handles TLS and Basic Auth, and Caddy never starts.

```
cp .env.example .env          # SCRAPER_PORT is the only value that matters here
docker compose -f docker-compose.yaml -f docker-compose.behind-proxy.yaml up -d
```

The scraper is now on `127.0.0.1:8081` and unreachable from outside the host.

For nginx, `nginx-site.conf.example` is a ready site config. Create the
password file, install the site, and let certbot add TLS:

```
apt-get install -y apache2-utils
htpasswd -c /etc/nginx/.htpasswd-scraper admin

cp nginx-site.conf.example /etc/nginx/sites-available/scraper
sed -i 's/SCRAPER_HOSTNAME/your.hostname.here/' /etc/nginx/sites-available/scraper
ln -s /etc/nginx/sites-available/scraper /etc/nginx/sites-enabled/scraper
nginx -t && systemctl reload nginx

certbot --nginx -d your.hostname.here
```

No spare domain? `sslip.io` resolves an embedded IP back to itself, so
`193-24-120-105.sslip.io` is a usable hostname that certbot will issue for.

Subsequent starts must repeat both `-f` flags, or Compose will fall back to
the Caddy layout:

```
docker compose -f docker-compose.yaml -f docker-compose.behind-proxy.yaml up -d
```

## Expectations

**Google blocks datacenter IP ranges.** A home connection scrapes fine; a VPS
often starts returning empty results within a few jobs. If that happens, the
cause is the IP, not the scraper — add residential proxies in the job form's
Proxies field, or in the `-proxies` flag.

**One job at a time.** Concurrency is pinned to 1 and the container is capped at
1400 MB, because a second concurrent Chromium is what takes a 2 GB host down.
Jobs queue rather than run in parallel, so this costs throughput, not results.
