# WireGuard + MongoDB VPN Setup

This project sets up:

- WireGuard VPN on an Ubuntu host
- Custom UDP VPN port (`12345` by default)
- VPN subnet `10.8.0.0/24`
- MongoDB in Docker
- A small Go management API in Docker
- Persistent MongoDB storage
- Scripts for installing/configuring WireGuard
- Peer creation through the API

> MongoDB is for management metadata. WireGuard itself does not require MongoDB.

## Requirements

- Ubuntu 22.04/24.04
- Root/sudo access
- Public IP or DNS name
- Docker + Docker Compose
- Cloud firewall/security group allowing UDP `12345`

## 1. Configure

Edit `.env`:

```env
VPN_PORT=12345
VPN_SUBNET=10.8.0.0/24
VPN_SERVER_IP=10.8.0.1
VPN_INTERFACE=wg0

MONGO_ROOT_USERNAME=vpnadmin
MONGO_ROOT_PASSWORD=CHANGE_ME_STRONG_PASSWORD

API_PORT=8080
```

Also edit `scripts/setup-wireguard.sh` if your Internet interface is not detected correctly.

## 2. Install WireGuard

```bash
chmod +x scripts/*.sh
sudo ./scripts/setup-wireguard.sh
```

The script installs WireGuard, enables IP forwarding, creates `/etc/wireguard/wg0.conf`, enables NAT, and opens the configured UDP port with UFW.

## 3. Start MongoDB + API

```bash
docker compose up -d --build
```

Check:

```bash
docker compose ps
curl http://127.0.0.1:8080/health
```

Expected:

```json
{"status":"ok"}
```

## 4. Create a VPN peer

```bash
curl -X POST http://127.0.0.1:8080/peers \
  -H 'Content-Type: application/json' \
  -d '{"name":"laptop-01"}'
```

The API returns the generated client configuration.

## 5. Check WireGuard

```bash
sudo wg show
```

After a client connects, look for:

```text
latest handshake
transfer
```

## 6. Cloud firewall

Allow:

```text
Protocol: UDP
Port: 12345
Source: your client networks or 0.0.0.0/0
```

Do not expose MongoDB port 27017 publicly.

## API

### Health

```http
GET /health
```

### Create peer

```http
POST /peers
Content-Type: application/json

{
  "name": "laptop-01"
}
```

### List peers

```http
GET /peers
```

### Delete peer

```http
DELETE /peers/{id}
```

## Security notes

This is a starting deployment, not a complete enterprise VPN management platform.

Before production:

- Put the API behind authentication.
- Use HTTPS for any remotely exposed API.
- Store secrets in Vault/secret manager instead of `.env`.
- Restrict the WireGuard UDP source range where practical.
- Back up MongoDB.
- Do not expose MongoDB directly to the Internet.
- Rotate/revoke peer keys when devices are lost.
