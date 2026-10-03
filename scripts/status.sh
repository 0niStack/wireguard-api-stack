#!/usr/bin/env bash
set -euo pipefail
sudo wg show
echo
sudo systemctl status wg-quick@wg0 --no-pager
