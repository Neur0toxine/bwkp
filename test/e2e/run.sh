#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
state="$root/test/e2e/.state"
compose=(docker compose -f "$root/test/e2e/compose.yml")
port=${BWKP_VAULTWARDEN_PORT:-18080}
backend_port=${BWKP_VAULTWARDEN_BACKEND_PORT:-18081}
server="https://localhost:$port"
email="bwkp-e2e@example.test"
import_email="bwkp-import-e2e@example.test"
master_password="E2E master password 2026!"
database_password="E2E database password 2026!"
totp_secret="JBSWY3DPEHPK3PXP"

cleanup() {
  if [[ -n "${waf_pid:-}" ]]; then
    kill "$waf_pid" >/dev/null 2>&1 || true
  fi
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_for_server() {
  for _ in $(seq 1 60); do
    if curl --cacert "$state/ca.pem" --fail --silent "$server/alive" >/dev/null; then
      return
    fi
    sleep 1
  done

  echo "Vaultwarden did not become ready" >&2
  return 1
}

# Prepare an isolated TLS-enabled Vaultwarden instance behind the header-checking
# proxy. All transient e2e data stays under test/e2e/.state.
rm -rf "$state"
mkdir -p "$state/vaultwarden" "$state/cli" "$state/output"
printf '%s\n' "$master_password" >"$state/master-password"
printf '%s\n' "$database_password" >"$state/database-password"
chmod 600 "$state/master-password" "$state/database-password"

openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
  -keyout "$state/ca-key.pem" \
  -out "$state/ca.pem" \
  -subj "/CN=bwkp e2e CA" \
  -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  >/dev/null 2>&1
openssl req -new -newkey rsa:2048 -nodes \
  -keyout "$state/vaultwarden/key.pem" \
  -out "$state/server.csr" \
  -subj "/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
  >/dev/null 2>&1
openssl x509 -req \
  -in "$state/server.csr" \
  -CA "$state/ca.pem" \
  -CAkey "$state/ca-key.pem" \
  -CAcreateserial \
  -days 1 \
  -copy_extensions copy \
  -out "$state/vaultwarden/cert.pem" \
  >/dev/null 2>&1

export NODE_EXTRA_CA_CERTS="$state/ca.pem"
export SSL_CERT_FILE="$state/ca.pem"
"${compose[@]}" up --detach --wait
node "$root/test/e2e/waf-proxy.mjs" "$port" "$backend_port" \
  "$state/vaultwarden/cert.pem" "$state/vaultwarden/key.pem" >"$state/waf.log" 2>&1 &
waf_pid=$!
wait_for_server

# Prove the proxy fixture rejects requests which do not identify themselves as
# the official Bitwarden client before testing bwkp's accepted requests.
waf_status=$(
  curl --cacert "$state/ca.pem" --silent --output /dev/null --write-out '%{http_code}' \
    "$server/attachments/test-cipher/test-file?token=test"
)
if [[ "$waf_status" != "401" ]]; then
  echo "e2e WAF did not reject an attachment request without a Bitwarden user-agent" >&2
  exit 1
fi
mutation_status=$(
  curl --cacert "$state/ca.pem" --silent --output /dev/null --write-out '%{http_code}' \
    --request POST "$server/api/ciphers"
)
if [[ "$mutation_status" != "401" ]]; then
  echo "e2e WAF did not reject a mutation without mandatory Bitwarden headers" >&2
  exit 1
fi

# Register source and destination users, then seed the source vault through the
# official CLI with representative records and attachments.
cargo run --quiet --manifest-path "$root/Cargo.toml" -p bwkp-e2e-register -- \
  "$server" "$email" "$master_password" "$state/ca.pem"
cargo run --quiet --manifest-path "$root/Cargo.toml" -p bwkp-e2e-register -- \
  "$server" "$import_email" "$master_password" "$state/ca.pem"
ssh-keygen -q -t ed25519 -N "" -C bwkp-e2e -f "$state/id_ed25519"
ssh_fingerprint=$(ssh-keygen -lf "$state/id_ed25519.pub" -E sha256 | awk '{print $2}')
node - \
  "$root/test/e2e/fixtures/vault.json" \
  "$state/vault.json" \
  "$state/id_ed25519" \
  "$state/id_ed25519.pub" \
  "$ssh_fingerprint" <<'NODE'
const fs = require("fs");
const [source, target, privatePath, publicPath, fingerprint] = process.argv.slice(2);
const vault = JSON.parse(fs.readFileSync(source, "utf8"));
const item = vault.items.find((candidate) => candidate.type === 5);
item.sshKey.privateKey = fs.readFileSync(privatePath, "utf8");
item.sshKey.publicKey = fs.readFileSync(publicPath, "utf8").trim();
item.sshKey.keyFingerprint = fingerprint;
fs.writeFileSync(target, JSON.stringify(vault));
NODE
export BITWARDENCLI_APPDATA_DIR="$state/cli"
bitwarden_cli_version=2026.6.0 # Keep aligned with native/bw/src/lib.rs.
bw=(npx -y "@bitwarden/cli@$bitwarden_cli_version")
"${bw[@]}" config server "$server" >/dev/null
session=$("${bw[@]}" login --raw --nointeraction "$email" "$master_password")
"${bw[@]}" --session "$session" import bitwardenjson "$state/vault.json" >/dev/null
"${bw[@]}" --session "$session" sync >/dev/null

item_id=$(
  "${bw[@]}" --session "$session" list items --search "Attachment Item" --raw |
    node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))'
)
"${bw[@]}" --session "$session" create attachment \
  --file "$root/test/e2e/fixtures/attachment.bin" --itemid "$item_id" >/dev/null
"${bw[@]}" --session "$session" create attachment \
  --file "$root/test/e2e/fixtures/attachment-second.bin" --itemid "$item_id" >/dev/null
archive_id=$(
  "${bw[@]}" --session "$session" list items --search "Archived Item" --raw |
    node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))'
)
delete_id=$(
  "${bw[@]}" --session "$session" list items --search "Deleted Item" --raw |
    node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))'
)
"${bw[@]}" --session "$session" archive item "$archive_id" >/dev/null
"${bw[@]}" --session "$session" delete item "$delete_id" >/dev/null
"${bw[@]}" logout >/dev/null

# Enable authenticator 2FA directly in the disposable test database so export
# also exercises the TOTP login path.
"${compose[@]}" stop vaultwarden >/dev/null
database="$state/vaultwarden/db.sqlite3"
user_id=$(sqlite3 "$database" "select uuid from users where email='$email';")
factor_id=$(cat /proc/sys/kernel/random/uuid)
sqlite3 "$database" \
  "insert into twofactor(uuid,user_uuid,atype,enabled,data,last_used) values('$factor_id','$user_id',0,1,'$totp_secret',0);"
"${compose[@]}" start vaultwarden >/dev/null
wait_for_server

go run "$root/test/e2e/totp" "$totp_secret" >"$state/totp"
chmod 600 "$state/totp"

# Export and inspect the KDBX independently with KeePassXC CLI.
"$root/dist/bwkp" export \
  --server "$server" \
  --email "$email" \
  --output "$state/output/vault.kdbx" \
  --master-password-file "$state/master-password" \
  --database-password-file "$state/database-password" \
  --totp-file "$state/totp" \
  --append-source \
  --kdf-memory-kib 8192 \
  --kdf-parallelism 1 \
  --kdf-iterations 10

if ! printf '%s\n' "$database_password" |
  keepassxc-cli add -q \
    --notes "Unsupported KeePassXC data preserved as a note." \
    "$state/output/vault.kdbx" \
    "Personal/Unfiled/Unsupported KeePassXC Entry"; then
  listing=$(printf '%s\n' "$database_password" | keepassxc-cli ls -q -R -f "$state/output/vault.kdbx")
  grep -F "Personal/Unfiled/Unsupported KeePassXC Entry" <<<"$listing" >/dev/null
fi

listing=$(printf '%s\n' "$database_password" | keepassxc-cli ls -q -R -f "$state/output/vault.kdbx")
expected_titles=(
  "Complex Login"
  "Secure Note"
  "Payment Card"
  "Full Identity"
  "SSH Key"
  "Attachment Item"
  "Archived Item"
  "Deleted Item"
  "Unsupported KeePassXC Entry"
)
for title in "${expected_titles[@]}"; do
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

# Import into the destination account and exercise every conflict mode.
mkdir -p "$state/import-cli"
export BITWARDENCLI_APPDATA_DIR="$state/import-cli"
"${bw[@]}" config server "$server" >/dev/null
import_session=$("${bw[@]}" login --raw --nointeraction "$import_email" "$master_password")

run_import() {
  local mode=$1
  "$root/dist/bwkp" import \
    --server "$server" \
    --email "$import_email" \
    --input "$state/output/vault.kdbx" \
    --master-password-file "$state/master-password" \
    --database-password-file "$state/database-password" \
    --conflict "$mode" \
    --no-progress
  "${bw[@]}" --session "$import_session" sync >/dev/null
}

run_import skip >"$state/import-initial.log"
grep -F "9 created" "$state/import-initial.log" >/dev/null
"${bw[@]}" --session "$import_session" list items --raw >"$state/imported-items.json"
"${bw[@]}" --session "$import_session" list items --trash --raw >"$state/imported-trash.json"
"${bw[@]}" --session "$import_session" list folders --raw >"$state/imported-folders.json"
node - "$state/imported-items.json" "$state/imported-folders.json" <<'NODE'
const fs = require("fs");
const [itemsPath, foldersPath] = process.argv.slice(2);
const items = JSON.parse(fs.readFileSync(itemsPath, "utf8"));
const folders = new Map(
  JSON.parse(fs.readFileSync(foldersPath, "utf8")).map((folder) => [folder.name, folder.id]),
);
const expectedFolders = [
  ["Complex Login", "Engineering/Production"],
  ["Secure Note", "Personal"],
  ["Attachment Item", "Unfiled"],
];
for (const [title, folder] of expectedFolders) {
  if (items.find((item) => item.name === title)?.folderId !== folders.get(folder)) {
    process.exit(1);
  }
}
NODE
run_import skip >"$state/import-skip.log"
grep -F "9 skipped" "$state/import-skip.log" >/dev/null

initial_login_id=$(
  "${bw[@]}" --session "$import_session" list items --search "Complex Login" --raw |
    node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))'
)
printf '%s\n' "$database_password" | keepassxc-cli edit -q --notes "Updated from KeePassXC" \
  "$state/output/vault.kdbx" "Personal/Engineering/Production/Complex Login"
run_import update >"$state/import-update.log"
grep -F "9 updated" "$state/import-update.log" >/dev/null
updated_login=$("${bw[@]}" --session "$import_session" list items --search "Complex Login" --raw)
node -e '
  const items = JSON.parse(process.argv[1]);
  if (items.length !== 1 || items[0].id !== process.argv[2] || items[0].notes !== "Updated from KeePassXC") {
    process.exit(1);
  }
' \
  "$updated_login" "$initial_login_id"

attachment_item=$("${bw[@]}" --session "$import_session" list items --search "Attachment Item" --raw)
node -e '
  const items = JSON.parse(process.argv[1]);
  if (items.length !== 1 || items[0].attachments?.length !== 2) process.exit(1);
' "$attachment_item"
"$root/dist/bwkp" export \
  --server "$server" \
  --email "$import_email" \
  --output "$state/output/imported-vault.kdbx" \
  --master-password-file "$state/master-password" \
  --database-password-file "$state/database-password" \
  --no-progress \
  --kdf-memory-kib 8192 \
  --kdf-parallelism 1 \
  --kdf-iterations 10 \
  >/dev/null
printf '%s\n' "$database_password" | keepassxc-cli attachment-export -q \
  "$state/output/imported-vault.kdbx" "Personal/Unfiled/Attachment Item" attachment.bin \
  "$state/output/imported-attachment.bin"
cmp "$root/test/e2e/fixtures/attachment.bin" "$state/output/imported-attachment.bin"

unsupported=$("${bw[@]}" --session "$import_session" list items --search "Unsupported KeePassXC Entry" --raw)
node -e '
  const items = JSON.parse(process.argv[1]);
  if (items.length !== 1 || items[0].type !== 2 || !items[0].notes.includes("preserved as a note")) {
    process.exit(1);
  }
' "$unsupported"
run_import delete >"$state/import-delete.log"
grep -F "9 replaced" "$state/import-delete.log" >/dev/null
replacement_login_id=$(
  "${bw[@]}" --session "$import_session" list items --search "Complex Login" --raw |
    node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(JSON.parse(s)[0].id))'
)
if [[ "$replacement_login_id" == "$initial_login_id" ]]; then
  echo "delete conflict mode did not replace the destination item" >&2
  exit 1
fi
trash=$("${bw[@]}" --session "$import_session" list items --trash --search "Complex Login" --raw)
node -e '
  const items = JSON.parse(process.argv[1]);
  if (!items.some((item) => item.id === process.argv[2])) process.exit(1);
' "$trash" "$initial_login_id"

before_duplicate=$(
  "${bw[@]}" --session "$import_session" list items --search "Complex Login" --raw |
    node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(String(JSON.parse(s).length)))'
)
run_import duplicate >"$state/import-duplicate.log"
grep -F "9 duplicated" "$state/import-duplicate.log" >/dev/null
after_duplicate=$(
  "${bw[@]}" --session "$import_session" list items --search "Complex Login" --raw |
    node -e 'let s="";process.stdin.on("data",d=>s+=d).on("end",()=>process.stdout.write(String(JSON.parse(s).length)))'
)
if (( after_duplicate != before_duplicate + 1 )); then
  echo "duplicate conflict mode did not create an additional item" >&2
  exit 1
fi
"${bw[@]}" logout >/dev/null
