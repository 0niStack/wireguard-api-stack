# WireGuard + MongoDB VPN Stack

Self-hosted WireGuard VPN on Ubuntu with a small Go management API and MongoDB (Docker) for peer metadata.

- WireGuard on the host, UDP port `12345`, subnet `10.8.0.0/24`
- Go API (Docker) creates, lists and deletes peers
- MongoDB (Docker) stores peer metadata with persistent storage

> WireGuard itself does not need MongoDB. Mongo only stores management data.

## Requirements

- Ubuntu 22.04 / 24.04 / 26.04 with sudo access
- Docker + Docker Compose
- Public IP or DNS name
- Security group / firewall allowing **UDP 12345**

## Quick start

### 1. Configure

Copy `.env.example` to `.env` and edit it:

```env
VPN_PORT=12345
VPN_SUBNET=10.8.0.0/24
VPN_SERVER_IP=10.8.0.1
VPN_INTERFACE=wg0
VPN_PUBLIC_ENDPOINT=your.server.ip.or.dns   # host only, no port

MONGO_ROOT_USERNAME=vpnadmin
MONGO_ROOT_PASSWORD=CHANGE_ME_STRONG_PASSWORD

API_PORT=8080
```

If your internet interface isn't detected correctly, also edit `scripts/setup-wireguard.sh`.

### 2. Install WireGuard on the host

```bash
chmod +x scripts/*.sh
sudo ./scripts/setup-wireguard.sh
```

This installs WireGuard, enables IP forwarding, creates `/etc/wireguard/wg0.conf`, sets up NAT, and opens the UDP port in UFW.

### 3. Start MongoDB and the API

```bash
docker compose up -d --build
curl http://127.0.0.1:8080/health     # {"status":"ok"}
```

### 4. Create a peer

```bash
curl -s -X POST http://127.0.0.1:8080/peers \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-laptop"}' | jq -r .config > my-laptop.conf
```

The config contains a private key. Copy it to the client, then delete it from the server.

### 5. Connect a client

Install WireGuard from [wireguard.com/install](https://www.wireguard.com/install/), import `my-laptop.conf`, and activate the tunnel.

Verify on the server:

```bash
sudo wg show wg0     # look for "latest handshake" on the peer
```

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/peers` | Create peer (`{"name":"laptop-01"}`), returns client config |
| GET | `/peers` | List peers |
| DELETE | `/peers/{id}` | Delete peer |

## Docker notes

- The API runs with `network_mode: host` and `cap_add: NET_ADMIN` so it can change the host's `wg0` interface.
- Mongo is published on `127.0.0.1:27017` only, for the host-networked API.
- Do **not** mount the host's `/usr/bin/wg` into the container. It is a glibc binary and won't run on Alpine; the image installs its own `wireguard-tools`.
- `GLIBC_TUNABLES=glibc.pthread.rseq=1` on the Mongo service is a workaround for MongoDB 8 crashing on Linux kernels 6.19 to 7.0.13 ([SERVER-121912](https://jira.mongodb.org/browse/SERVER-121912)). Remove it once the host kernel is 7.0.14 or newer.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `go mod download` checksum mismatch | Delete the bad lines from `api/go.sum`, run `go mod tidy` in `api/`, commit the result |
| `vpn-mongodb` restarting, log says kernel incompatibility | Keep the `GLIBC_TUNABLES` workaround or upgrade the kernel |
| `failed to generate key` | Remove the `/usr/bin/wg` volume mounts and rebuild |
| `Unable to modify interface: Operation not permitted` | Use `network_mode: host` and `cap_add: NET_ADMIN` |
| Config shows `YOUR_PUBLIC_IP_OR_DNS` | Set `VPN_PUBLIC_ENDPOINT` in `.env`, then `docker compose up -d api` |
| No handshake | Open UDP 12345 in the cloud firewall and check the `Endpoint` in the client config |
| Connected but no internet | Check `sysctl net.ipv4.ip_forward` is `1` and the NAT rule exists |

## Security notes

This is a starting deployment, not a full VPN management platform.

- The API has no authentication and, with host networking, listens on all interfaces. **Do not open TCP 8080** to the internet; put it behind auth and HTTPS before exposing it.
- Never expose MongoDB (27017) publicly.
- Keep secrets out of git; use a secret manager instead of `.env` for production.
- Restrict the WireGuard UDP source range where practical.
- Back up MongoDB and revoke peers when a device is lost.
