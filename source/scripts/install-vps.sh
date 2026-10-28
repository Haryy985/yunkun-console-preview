#!/usr/bin/env bash
# Fresh Alibaba Cloud Linux 3 installation from a reviewed, adjacent ZIP.
set -Eeuo pipefail
umask 077

readonly ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly INSTALL="/opt/yunkun-console"
readonly ACCOUNT="yunkun-console"
readonly SERVICE="yunkun-console"
readonly CERTBOT="/opt/yunkun-certbot/bin/certbot"
readonly WEBROOT="/var/www/yunkun-acme"
readonly NGINX_CONF="/etc/nginx/conf.d/yunkun-console.conf"
PUBLIC_IP="${1:-}"
ARCHIVE=""
STEP="preflight"

fail() { printf 'FAILED [%s]: %s\n' "$STEP" "$*" >&2; exit 1; }
on_error() {
    local line="$1"
    printf 'FAILED [%s] at line %s. Existing files were preserved; do not re-run over a partial installation.\n' "$STEP" "$line" >&2
}
trap 'on_error "$LINENO"' ERR
info() { printf '[%s] %s\n' "$STEP" "$*"; }

[[ "$(id -u)" == 0 ]] || fail "run this installer as root"
[[ -t 0 ]] || fail "an interactive terminal is required for the administrator password"
[[ $# -le 1 ]] || fail "usage: bash install-vps.sh [public-ip]"
[[ -r /etc/os-release ]] || fail "missing /etc/os-release"
# shellcheck disable=SC1091
source /etc/os-release
[[ "${ID:-}" == "alinux" && "${VERSION_ID:-}" == "3" ]] ||
    fail "this installer is tested only for Alibaba Cloud Linux 3"
case "$(uname -m)" in
    x86_64) PLATFORM="linux-amd64" ;;
    aarch64) PLATFORM="linux-arm64" ;;
    *) fail "unsupported CPU architecture" ;;
esac
[[ ! -e "$INSTALL" && ! -L "$INSTALL" ]] || fail "$INSTALL exists; existing data must never be overwritten"
[[ ! -e "$NGINX_CONF" && ! -L "$NGINX_CONF" ]] || fail "$NGINX_CONF exists"
[[ ! -e /etc/systemd/system/yunkun-console.service ]] || fail "service already exists"
[[ ! -e /etc/systemd/system/yunkun-certbot-renew.timer ]] || fail "renewal timer already exists"
[[ ! -e /etc/systemd/system/yunkun-certbot-renew.service ]] || fail "renewal service already exists"
[[ ! -e /etc/letsencrypt/renewal-hooks/deploy/reload-yunkun-nginx.sh ]] || fail "renewal hook already exists"
[[ ! -e /opt/yunkun-certbot ]] || fail "Certbot environment already exists"
[[ ! -e "$WEBROOT" && ! -L "$WEBROOT" ]] || fail "ACME webroot already exists"
! id "$ACCOUNT" >/dev/null 2>&1 || fail "service account already exists"
! systemctl is-active --quiet nginx || fail "Nginx already runs; refusing to change an existing site"
dnf install -y iproute gawk coreutils >/dev/null
for port in 80 443 5173 5433; do
    if ss -H -ltn "( sport = :$port )" | grep -q .; then
        fail "port $port is occupied; refusing to stop an existing service"
    fi
done
[[ -r "$ROOT/SHA256SUMS" && ! -L "$ROOT/SHA256SUMS" ]] || fail "missing SHA256SUMS"
shopt -s nullglob
archives=("$ROOT"/*-"$PLATFORM".zip)
[[ ${#archives[@]} == 1 ]] || fail "place exactly one $PLATFORM ZIP beside this script"
ARCHIVE="${archives[0]}"
[[ -f "$ARCHIVE" && ! -L "$ARCHIVE" ]] || fail "archive is not a regular file"
name="${ARCHIVE##*/}"
expected="$(awk -v name="$name" '$2 == name && $1 ~ /^[a-fA-F0-9]+$/ { print $1 }' "$ROOT/SHA256SUMS")"
[[ "$expected" =~ ^[a-fA-F0-9]{64}$ ]] || fail "missing or duplicate archive hash"
printf '%s  %s\n' "$expected" "$ARCHIVE" | sha256sum -c - >/dev/null ||
    fail "archive SHA-256 mismatch"
info "verified $name for $PLATFORM"

STEP="dependencies"
dnf install -y nginx python3.11 curl unzip shadow-utils util-linux >/dev/null
python3.11 -m venv /opt/yunkun-certbot
/opt/yunkun-certbot/bin/pip install --disable-pip-version-check 'certbot>=5.4,<7' >/dev/null
"$CERTBOT" --help all | grep -- '--ip-address' >/dev/null || fail "Certbot lacks IP certificate support"

if [[ -z "$PUBLIC_IP" ]]; then
    token="$(curl --noproxy '*' -fsS --max-time 3 -X PUT \
        -H 'X-aliyun-ecs-metadata-token-ttl-seconds: 60' \
        'http://100.100.100.200/latest/api/token' 2>/dev/null || true)"
    if [[ -n "$token" ]]; then
        for key in eipv4 public-ipv4; do
            PUBLIC_IP="$(curl --noproxy '*' -fsS --max-time 3 \
                -H "X-aliyun-ecs-metadata-token: $token" \
                "http://100.100.100.200/latest/meta-data/$key" 2>/dev/null || true)"
            [[ -n "$PUBLIC_IP" ]] && break
        done
    fi
fi
if [[ -z "$PUBLIC_IP" ]]; then
    read -r -p "Public IPv4 address: " PUBLIC_IP
fi
python3.11 - "$PUBLIC_IP" <<'PY' || fail "public IPv4 address is invalid"
import ipaddress
import sys
ip = ipaddress.ip_address(sys.argv[1])
if ip.version != 4 or not ip.is_global:
    raise SystemExit(1)
PY
info "public IPv4: $PUBLIC_IP"

read -r -p "Certificate contact email: " EMAIL
[[ "$EMAIL" == *@*.* && "$EMAIL" != *$'\n'* ]] || fail "invalid contact email"
read -r -s -p "New console administrator password (at least 16 characters): " PASSWORD
printf '\n'
[[ ${#PASSWORD} -ge 16 && ${#PASSWORD} -le 72 ]] || fail "password length must be 16-72 characters"
read -r -s -p "Confirm administrator password: " CONFIRM
printf '\n'
[[ "$PASSWORD" == "$CONFIRM" ]] || fail "password confirmation differs"
unset CONFIRM

STEP="certificate"
if [[ ! -e /var/www ]]; then
    install -d -m 755 /var/www
fi
runuser -u nginx -- test -x /var/www || fail "Nginx cannot traverse /var/www"
mkdir -p "$WEBROOT/.well-known/acme-challenge"
chmod 755 "$WEBROOT" "$WEBROOT/.well-known" "$WEBROOT/.well-known/acme-challenge"
cat > "$NGINX_CONF" <<EOF
server {
    listen 80;
    server_name $PUBLIC_IP;
    location ^~ /.well-known/acme-challenge/ { root $WEBROOT; }
    location / { return 403; }
}
EOF
nginx -t
systemctl enable --now nginx
if systemctl is-active --quiet firewalld; then
    firewall-cmd --permanent --add-service=http >/dev/null
    firewall-cmd --reload >/dev/null
fi
info "Port 80 must be open in the Alibaba Cloud security group for ACME validation"
"$CERTBOT" certonly --webroot -w "$WEBROOT" \
    --preferred-profile shortlived --ip-address "$PUBLIC_IP" \
    --agree-tos --no-eff-email --non-interactive --email "$EMAIL"
[[ -s "/etc/letsencrypt/live/$PUBLIC_IP/fullchain.pem" &&
   -s "/etc/letsencrypt/live/$PUBLIC_IP/privkey.pem" ]] || fail "certificate missing"

STEP="installation"
# The extractor rejects private paths, links, duplicates, unexpected roots,
# oversized entries, and a mismatched internal per-file hash manifest.
python3.11 - "$ARCHIVE" "$INSTALL" "$PLATFORM" <<'PY'
import hashlib
import json
from pathlib import Path, PurePosixPath
import stat
import sys
import zipfile

archive, destination, platform = Path(sys.argv[1]), Path(sys.argv[2]), sys.argv[3]
private = {".yunkun-local", ".git", ".env", "config.json",
           "installation.json", "postgres-data", "jwt.key", "proxy-pool.key"}
with zipfile.ZipFile(archive) as z:
    entries, seen, total, roots = [], set(), 0, set()
    if len(z.infolist()) > 25000:
        raise ValueError("too many archive entries")
    for item in z.infolist():
        parts = item.filename.rstrip("/").split("/")
        if len(parts) < 2 or any(p in ("", ".", "..") or "\\" in p or ":" in p for p in parts):
            raise ValueError("unsafe archive path")
        roots.add(parts[0])
        rel = PurePosixPath(*parts[1:])
        if any(p.lower() in private or p.lower().startswith(".yunkun-local") for p in rel.parts):
            raise ValueError("private archive entry")
        key = str(rel).casefold()
        if key in seen or item.is_dir():
            raise ValueError("duplicate or unexpected directory entry")
        seen.add(key)
        mode = item.external_attr >> 16
        if stat.S_ISLNK(mode) or stat.S_IFMT(mode) not in (0, stat.S_IFREG):
            raise ValueError("links and special files are forbidden")
        total += item.file_size
        if item.file_size > 1 << 30 or total > 4 << 30:
            raise ValueError("oversized package")
        entries.append((item, rel, mode & 0o755))
    if len(roots) != 1 or not roots.pop().startswith("yunkun-v"):
        raise ValueError("unexpected archive root")
    for required in ("bin/yunkun", "bin/yunkun-offline", "runtime/bin/postgres",
                     "runtime/bin/initdb", "runtime/bin/pg_ctl", "PACKAGE.json",
                     "VERSION", "FILES.sha256", "start.sh", "check.sh", "stop.sh"):
        if required.casefold() not in seen:
            raise ValueError("missing required file: " + required)
    manifest = json.loads(z.read(next(i for i, rel, _ in entries if str(rel) == "PACKAGE.json")))
    if manifest["platform"] != platform or manifest["data_included"] is not False:
        raise ValueError("package platform or privacy mismatch")
    version = z.read(next(i for i, rel, _ in entries if str(rel) == "VERSION")).decode().strip()
    if manifest["version"] != version or version != "v1.1.3-offline":
        raise ValueError("package version mismatch")
    checks = {
        name: digest for digest, name in (
            line.split("  ", 1)
            for line in z.read(next(i for i, rel, _ in entries if str(rel) == "FILES.sha256"))
                .decode().splitlines()
        )
    }
    if set(checks) != {str(rel) for _, rel, _ in entries if str(rel) != "FILES.sha256"}:
        raise ValueError("internal file list differs")
    destination.mkdir(mode=0o700)
    for item, rel, mode in entries:
        target = destination.joinpath(*rel.parts)
        target.parent.mkdir(parents=True, exist_ok=True)
        digest = hashlib.sha256()
        with z.open(item) as src, target.open("xb") as dst:
            while chunk := src.read(1024 * 1024):
                dst.write(chunk)
                digest.update(chunk)
        if str(rel) != "FILES.sha256" and digest.hexdigest() != checks[str(rel)]:
            raise ValueError("file hash mismatch: " + str(rel))
        target.chmod(mode or 0o600)
PY
useradd --system --home-dir "$INSTALL" --shell /sbin/nologin "$ACCOUNT"
chown -R "$ACCOUNT:$ACCOUNT" "$INSTALL"
runuser -u "$ACCOUNT" -- "$INSTALL/check.sh"
cat > "/etc/systemd/system/$SERVICE.service" <<EOF
[Unit]
Description=Yunkun private offline console
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$ACCOUNT
Group=$ACCOUNT
WorkingDirectory=$INSTALL
Environment=HOME=$INSTALL
ExecStart=$INSTALL/start.sh --no-browser
ExecStop=$INSTALL/stop.sh
Restart=on-failure
RestartSec=5
TimeoutStartSec=180
TimeoutStopSec=120
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now "$SERVICE"
ready=0
for ((i=0; i<120; i++)); do
    if curl --noproxy '*' -fsS --max-time 2 http://127.0.0.1:5173/api/health |
        python3.11 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("ready") else 1)' 2>/dev/null; then
        ready=1
        break
    fi
    sleep 1
done
[[ "$ready" == 1 ]] || fail "console not ready; inspect journalctl -u $SERVICE"
# First-user initialization is completed on loopback before Nginx exposes setup.
status="$(printf '%s' "$PASSWORD" |
    python3.11 -c 'import json,sys; print(json.dumps({"password":sys.stdin.read()}))' |
    curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 20 \
        -H 'Content-Type: application/json' --data-binary @- \
        http://127.0.0.1:5173/api/auth/init)"
unset PASSWORD
[[ "$status" == 200 ]] || fail "administrator initialization failed (HTTP $status); public proxy remains disabled"
info "administrator initialized locally before public access"

STEP="public-https"
cat > "$NGINX_CONF" <<EOF
map \$http_upgrade \$yunkun_connection_upgrade {
    default upgrade;
    '' close;
}
server {
    listen 80;
    server_name $PUBLIC_IP;
    location ^~ /.well-known/acme-challenge/ { root $WEBROOT; }
    location / { return 308 https://$PUBLIC_IP\$request_uri; }
}
server {
    listen 443 ssl;
    server_name $PUBLIC_IP;
    ssl_certificate /etc/letsencrypt/live/$PUBLIC_IP/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$PUBLIC_IP/privkey.pem;
    location / {
        proxy_pass http://127.0.0.1:5173;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$yunkun_connection_upgrade;
        proxy_read_timeout 3600s;
        proxy_buffering off;
    }
}
EOF
nginx -t
systemctl reload nginx
if systemctl is-active --quiet firewalld; then
    firewall-cmd --permanent --add-service=https >/dev/null
    firewall-cmd --reload >/dev/null
fi
curl --noproxy '*' --resolve "$PUBLIC_IP:443:127.0.0.1" -fsS --max-time 10 \
    "https://$PUBLIC_IP/api/health" |
    python3.11 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("ready") else 1)'

STEP="renewal"
mkdir -p /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/reload-yunkun-nginx.sh <<'EOF'
#!/bin/sh
/usr/sbin/nginx -t >/dev/null && /usr/bin/systemctl reload nginx
EOF
chmod 755 /etc/letsencrypt/renewal-hooks/deploy/reload-yunkun-nginx.sh
cat > /etc/systemd/system/yunkun-certbot-renew.service <<EOF
[Unit]
Description=Renew Yunkun IP certificate

[Service]
Type=oneshot
ExecStart=$CERTBOT renew --quiet --cert-name $PUBLIC_IP
EOF
cat > /etc/systemd/system/yunkun-certbot-renew.timer <<'EOF'
[Unit]
Description=Check Yunkun IP certificate twice daily

[Timer]
OnCalendar=*-*-* 00,12:00:00
RandomizedDelaySec=30m
Persistent=true

[Install]
WantedBy=timers.target
EOF
systemctl daemon-reload
systemctl enable --now yunkun-certbot-renew.timer
"$CERTBOT" renew --dry-run --cert-name "$PUBLIC_IP" --run-deploy-hooks

STEP="complete"
printf '\nREADY: https://%s/login/\n' "$PUBLIC_IP"
printf 'Admin username: yunkun; password is the one entered in this installer.\n'
printf 'Cloud prerequisite: allow inbound TCP 443 for your client IP and TCP 80 for certificate renewal.\n'
printf 'App and private PostgreSQL remain bound to loopback. Preserve %s/.yunkun-local.\n' "$INSTALL"
