#!/bin/bash
set -e

PROXY_IP="${PROXY_IP:-10.0.0.100}"
PROXY_PORT="${PROXY_PORT:-3128}"

cat > /etc/environment <<EOF
http_proxy=http://${PROXY_IP}:${PROXY_PORT}
https_proxy=http://${PROXY_IP}:${PROXY_PORT}
HTTP_PROXY=http://${PROXY_IP}:${PROXY_PORT}
HTTPS_PROXY=http://${PROXY_IP}:${PROXY_PORT}
no_proxy=localhost,127.0.0.1,10.0.0.0/8
NO_PROXY=localhost,127.0.0.1,10.0.0.0/8
EOF

iptables -F OUTPUT
iptables -A OUTPUT -p tcp --dport 80 -d ${PROXY_IP} -j ACCEPT
iptables -A OUTPUT -p tcp --dport 443 -d ${PROXY_IP} -j ACCEPT
iptables -A OUTPUT -p tcp --dport 80 -j DROP
iptables -A OUTPUT -p tcp --dport 443 -j DROP
iptables -A OUTPUT -j ACCEPT

iptables-save > /etc/iptables/rules.v4

echo "Proxy configuration complete. Proxy: ${PROXY_IP}:${PROXY_PORT}"
