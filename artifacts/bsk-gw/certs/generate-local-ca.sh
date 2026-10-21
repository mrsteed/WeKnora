#!/usr/bin/env bash
set -euo pipefail

cert_dir=$(cd "$(dirname "$0")" && pwd)
primary_ip=${1:-192.168.1.188}
root_cn=${2:-WeKnora BrowserSkill Local CA}

backup_dir="$cert_dir/backups/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup_dir"

for path in \
  "$cert_dir/ca.cer" \
  "$cert_dir/ca.pem" \
  "$cert_dir/ca-key.pem" \
  "$cert_dir/cert.cer" \
  "$cert_dir/cert.pem" \
  "$cert_dir/key.pem" \
  "$cert_dir/fullchain.pem" \
  "$cert_dir/server.csr" \
  "$cert_dir/ca.srl"
do
  if [[ -f "$path" ]]; then
    cp "$path" "$backup_dir/$(basename "$path")"
  fi
done

root_conf=$(mktemp)
server_conf=$(mktemp)
trap 'rm -f "$root_conf" "$server_conf"' EXIT

cat >"$root_conf" <<EOF
[req]
distinguished_name = dn
x509_extensions = v3_ca
prompt = no

[dn]
CN = $root_cn

[v3_ca]
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid:always,issuer
EOF

cat >"$server_conf" <<EOF
[req]
distinguished_name = dn
req_extensions = v3_req
prompt = no

[dn]
CN = $primary_ip

[v3_req]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[v3_leaf]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
authorityKeyIdentifier = keyid,issuer
subjectKeyIdentifier = hash

[alt_names]
IP.1 = $primary_ip
IP.2 = 127.0.0.1
DNS.1 = localhost
EOF

openssl genrsa -out "$cert_dir/ca-key.pem" 4096 >/dev/null 2>&1
openssl req -x509 -new -sha256 -days 3650 \
  -key "$cert_dir/ca-key.pem" \
  -out "$cert_dir/ca.pem" \
  -config "$root_conf" >/dev/null 2>&1

openssl genrsa -out "$cert_dir/key.pem" 2048 >/dev/null 2>&1
openssl req -new \
  -key "$cert_dir/key.pem" \
  -out "$cert_dir/server.csr" \
  -config "$server_conf" >/dev/null 2>&1
openssl x509 -req -sha256 -days 825 \
  -in "$cert_dir/server.csr" \
  -CA "$cert_dir/ca.pem" \
  -CAkey "$cert_dir/ca-key.pem" \
  -CAcreateserial \
  -out "$cert_dir/cert.pem" \
  -extfile "$server_conf" \
  -extensions v3_leaf >/dev/null 2>&1

cat "$cert_dir/cert.pem" "$cert_dir/ca.pem" > "$cert_dir/fullchain.pem"
openssl x509 -in "$cert_dir/ca.pem" -outform der -out "$cert_dir/ca.cer"
openssl x509 -in "$cert_dir/cert.pem" -outform der -out "$cert_dir/cert.cer"

echo "Generated files:"
echo "  CA root DER : $cert_dir/ca.cer"
echo "  CA root     : $cert_dir/ca.pem"
echo "  CA key      : $cert_dir/ca-key.pem"
echo "  Server cert DER: $cert_dir/cert.cer"
echo "  Server cert : $cert_dir/cert.pem"
echo "  Server key  : $cert_dir/key.pem"
echo "  Full chain  : $cert_dir/fullchain.pem"
echo ""
echo "Import ONLY ca.pem into Windows Trusted Root Certification Authorities."
echo "Do NOT import cert.pem into the root store."