#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state="$root/test/e2e/.state"
compose=(docker compose -f "$root/test/e2e/compose.yml")
port=${BWKP_VAULTWARDEN_PORT:-18080}
server="https://localhost:$port"
email="bwkp-e2e@example.test"
master_password="E2E master password 2026!"
database_password="E2E database password 2026!"
totp_secret="JBSWY3DPEHPK3PXP"

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

rm -rf "$state"
mkdir -p "$state/vaultwarden" "$state/cli" "$state/output"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
  -keyout "$state/ca-key.pem" -out "$state/ca.pem" -subj "/CN=bwkp e2e CA" \
  -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" >/dev/null 2>&1
openssl req -new -newkey rsa:2048 -nodes -keyout "$state/vaultwarden/key.pem" \
  -out "$state/server.csr" -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" >/dev/null 2>&1
openssl x509 -req -in "$state/server.csr" -CA "$state/ca.pem" -CAkey "$state/ca-key.pem" \
  -CAcreateserial -days 1 -copy_extensions copy -out "$state/vaultwarden/cert.pem" >/dev/null 2>&1
export NODE_EXTRA_CA_CERTS="$state/ca.pem"
export SSL_CERT_FILE="$state/ca.pem"
"${compose[@]}" up --detach --wait

cargo run --quiet --manifest-path "$root/Cargo.toml" -p bwkp-e2e-register -- \
  "$server" "$email" "$master_password" "$state/ca.pem"
ssh-keygen -q -t ed25519 -N "" -C bwkp-e2e -f "$state/id_ed25519"
ssh_fingerprint=$(ssh-keygen -lf "$state/id_ed25519.pub" -E sha256 | awk '{print $2}')
node - "$root/test/e2e/fixtures/vault.json" "$state/vault.json" "$state/id_ed25519" "$state/id_ed25519.pub" "$ssh_fingerprint" <<'NODE'
const fs = require("fs");
const [source, target, privatePath, publicPath, fingerprint] = process.argv.slice(2);
const vault = JSON.parse(fs.readFileSync(source, "utf8"));
const item = vault.items.find(candidate => candidate.type === 5);
item.sshKey.privateKey = fs.readFileSync(privatePath, "utf8");
item.sshKey.publicKey = fs.readFileSync(publicPath, "utf8").trim();
item.sshKey.keyFingerprint = fingerprint;
fs.writeFileSync(target, JSON.stringify(vault));
NODE
export BITWARDENCLI_APPDATA_DIR="$state/cli"
bw=(npx -y @bitwarden/cli@2026.6.0)
"${bw[@]}" config server "$server" >/dev/null
session=$("${bw[@]}" login --raw --nointeraction "$email" "$master_password")
"${bw[@]}" --session "$session" import bitwardenjson "$state/vault.json" >/dev/null
"${bw[@]}" --session "$session" sync >/dev/null

item_id=$("${bw[@]}" --session "$session" list items --search "Attachment Item" --raw | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))')
"${bw[@]}" --session "$session" create attachment --file "$root/test/e2e/fixtures/attachment.bin" --itemid "$item_id" >/dev/null
archive_id=$("${bw[@]}" --session "$session" list items --search "Archived Item" --raw | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))')
delete_id=$("${bw[@]}" --session "$session" list items --search "Deleted Item" --raw | node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))')
"${bw[@]}" --session "$session" archive item "$archive_id" >/dev/null
"${bw[@]}" --session "$session" delete item "$delete_id" >/dev/null
"${bw[@]}" logout >/dev/null

"${compose[@]}" stop vaultwarden >/dev/null
database="$state/vaultwarden/db.sqlite3"
user_id=$(sqlite3 "$database" "select uuid from users where email='$email';")
factor_id=$(cat /proc/sys/kernel/random/uuid)
sqlite3 "$database" "insert into twofactor(uuid,user_uuid,atype,enabled,data,last_used) values('$factor_id','$user_id',0,1,'$totp_secret',0);"
"${compose[@]}" start vaultwarden >/dev/null
for _ in $(seq 1 60); do
  if curl --cacert "$state/ca.pem" --fail --silent "$server/alive" >/dev/null; then break; fi
  sleep 1
done

printf '%s\n' "$master_password" >"$state/master-password"
printf '%s\n' "$database_password" >"$state/database-password"
go run "$root/test/e2e/totp" "$totp_secret" >"$state/totp"
chmod 600 "$state/master-password" "$state/database-password" "$state/totp"

"$root/dist/bwkp" export \
  --server "$server" --email "$email" --output "$state/output/vault.kdbx" \
  --master-password-file "$state/master-password" --database-password-file "$state/database-password" \
  --totp-file "$state/totp" --kdf-memory-kib 8192 --kdf-parallelism 1 --kdf-iterations 10

listing=$(printf '%s\n' "$database_password" | keepassxc-cli ls -q -R -f "$state/output/vault.kdbx")
for title in "Complex Login" "Secure Note" "Payment Card" "Full Identity" "SSH Key" "Attachment Item" "Archived Item" "Deleted Item"; do
  grep -F "$title" <<<"$listing" >/dev/null
done
printf '%s\n' "$database_password" | keepassxc-cli db-info -q "$state/output/vault.kdbx" >/dev/null

login=$(printf '%s\n' "$database_password" | keepassxc-cli show -q --all --show-protected \
  "$state/output/vault.kdbx" "Personal/Engineering/Production/Complex Login")
grep -F "UserName: alice@example.test" <<<"$login" >/dev/null
grep -F "Password: correct horse battery staple" <<<"$login" >/dev/null
grep -F "Environment: production" <<<"$login" >/dev/null
grep -F "KP2A_URL_1: https://admin.example.test" <<<"$login" >/dev/null
grep -F "otp: otpauth://" <<<"$login" >/dev/null

printf '%s\n' "$database_password" | keepassxc-cli attachment-export -q \
  "$state/output/vault.kdbx" "Personal/Unfiled/Attachment Item" attachment.bin \
  "$state/output/attachment.bin"
cmp "$root/test/e2e/fixtures/attachment.bin" "$state/output/attachment.bin"
