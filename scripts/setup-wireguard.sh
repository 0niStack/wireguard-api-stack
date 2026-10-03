#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$PROJECT_DIR/.env"

if [[ ! -f "$ENV_FILE" ]]; then
  cp "$PROJECT_DIR/.env.example" "$ENV_FILE"
  echo "Created .env from .env.example"
  echo "Edit .env before running this script."
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

if [[ $EUID -ne 0 ]]; then
  echo "Run with sudo: sudo $0"
  exit 1
fi

apt-get update
apt-get install -y wireguard iptables ufw

WAN_IFACE="$(ip route | awk '/default/ {print $5; exit}')"
if [[ -z "$WAN_IFACE" ]]; then
  echo "Could not detect WAN interface."
  exit 1
fi

echo "Detected WAN interface: $WAN_IFACE"

mkdir -p /etc/wireguard
chmod 700 /etc/wireguard

if [[ ! -f /etc/wireguard/server_private.key ]]; then
  wg genkey | tee /etc/wireguard/server_private.key | wg pubkey > /etc/wireguard/server_public.key
  chmod 600 /etc/wireguard/server_private.key
fi

SERVER_PRIVATE_KEY="$(cat /etc/wireguard/server_private.key)"

cat > /etc/wireguard/wg0.conf <<EOF
[Interface]
Address = ${VPN_SERVER_IP}/24
ListenPort = ${VPN_PORT}
PrivateKey = ${SERVER_PRIVATE_KEY}

PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -A FORWARD -o wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o ${WAN_IFACE} -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -D FORWARD -o wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o ${WAN_IFACE} -j MASQUERADE
EOF

chmod 600 /etc/wireguard/wg0.conf

cat > /etc/sysctl.d/99-wireguard.conf <<EOF
net.ipv4.ip_forward=1
net.ipv6.conf.all.forwarding=1
EOF

sysctl --system

ufw allow "${VPN_PORT}/udp"
ufw allow OpenSSH
ufw --force enable

systemctl enable wg-quick@wg0
systemctl restart wg-quick@wg0

echo
echo "WireGuard is ready."
echo "Interface: wg0"
echo "UDP port: ${VPN_PORT}"
echo "Server VPN IP: ${VPN_SERVER_IP}"
echo "Server public key:"
cat /etc/wireguard/server_public.key
echo
wg show
