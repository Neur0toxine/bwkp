package e2e

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- RFC 6238 interoperability requires HMAC-SHA-1.
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	sourceEmail       = "bwkp-e2e@example.test"
	destinationEmail  = "bwkp-import-e2e@example.test"
	masterPassword    = "E2E master password 2026!"
	databasePassword  = "E2E database password 2026!"
	totpSecret        = "JBSWY3DPEHPK3PXP"
	bitwardenVersion  = "2026.6.0" // Keep aligned with native/bw/src/lib.rs.
	commandTimeout    = 10 * time.Minute
	serverWaitTimeout = time.Minute
)

type testEnvironment struct {
	root         string
	state        string
	server       string
	caPool       *x509.CertPool
	httpClient   *http.Client
	cliData      string
	importData   string
	output       string
	masterFile   string
	databaseFile string
	totpFile     string
}

func newTestEnvironment(t *testing.T) *testEnvironment {
	t.Helper()

	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate e2e helper source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	state := filepath.Join(root, "test", "e2e", ".state")
	if err := os.RemoveAll(state); err != nil {
		t.Fatalf("remove old e2e state: %v", err)
	}
	for _, directory := range []string{
		filepath.Join(state, "vaultwarden"),
		filepath.Join(state, "cli"),
		filepath.Join(state, "import-cli"),
		filepath.Join(state, "output"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create e2e state directory: %v", err)
		}
	}

	environment := &testEnvironment{
		root:         root,
		state:        state,
		server:       "https://localhost:" + envOr("BWKP_VAULTWARDEN_PORT", "18080"),
		cliData:      filepath.Join(state, "cli"),
		importData:   filepath.Join(state, "import-cli"),
		output:       filepath.Join(state, "output"),
		masterFile:   filepath.Join(state, "master-password"),
		databaseFile: filepath.Join(state, "database-password"),
		totpFile:     filepath.Join(state, "totp"),
	}
	environment.writeSecret(t, environment.masterFile, masterPassword)
	environment.writeSecret(t, environment.databaseFile, databasePassword)
	environment.generateCertificates(t)
	t.Cleanup(func() { environment.composeCleanup(t) })
	environment.compose(t, "up", "--detach", "--wait")
	environment.startWAF(t)
	environment.waitForServer(t)
	return environment
}

func (environment *testEnvironment) generateCertificates(t *testing.T) {
	t.Helper()

	now := time.Now()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: "bwkp e2e CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: randomSerial(t),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	writeFile(t, filepath.Join(environment.state, "ca.pem"), caPEM, 0o644)
	writeFile(t, filepath.Join(environment.state, "vaultwarden", "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), 0o644)
	writeFile(t, filepath.Join(environment.state, "vaultwarden", "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}), 0o600)

	environment.caPool = x509.NewCertPool()
	if !environment.caPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("load generated CA certificate")
	}
	environment.httpClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: environment.caPool, MinVersion: tls.VersionTLS12}},
	}
}

func (environment *testEnvironment) startWAF(t *testing.T) {
	t.Helper()

	backend, err := url.Parse("https://127.0.0.1:" + envOr("BWKP_VAULTWARDEN_BACKEND_PORT", "18081"))
	if err != nil {
		t.Fatalf("parse Vaultwarden backend URL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(backend)
	proxy.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: environment.caPool, MinVersion: tls.VersionTLS12}}
	wafLog, err := os.OpenFile(filepath.Join(environment.state, "waf.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create WAF log: %v", err)
	}
	proxy.ErrorLog = log.New(wafLog, "", log.LstdFlags)

	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mutation := (request.Method == http.MethodPost || request.Method == http.MethodPut || request.Method == http.MethodDelete) &&
			(strings.HasPrefix(request.URL.Path, "/api/ciphers") || strings.HasPrefix(request.URL.Path, "/api/folders"))
		attachment := strings.HasPrefix(request.URL.Path, "/attachments/")
		official := strings.HasPrefix(request.UserAgent(), "Bitwarden_CLI/") &&
			request.Header.Get("Bitwarden-Client-Name") == "cli" &&
			request.Header.Get("Bitwarden-Client-Version") != "" &&
			request.Header.Get("Device-Type") != ""
		if (mutation || attachment) && !official {
			http.Error(response, "request rejected by e2e WAF: official client headers are required", http.StatusUnauthorized)
			return
		}
		proxy.ServeHTTP(response, request)
	})

	certificate, err := tls.LoadX509KeyPair(
		filepath.Join(environment.state, "vaultwarden", "cert.pem"),
		filepath.Join(environment.state, "vaultwarden", "key.pem"),
	)
	if err != nil {
		t.Fatalf("load WAF certificate: %v", err)
	}
	port := envOr("BWKP_VAULTWARDEN_PORT", "18080")
	listener, err := tls.Listen("tcp", "127.0.0.1:"+port, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatalf("start e2e WAF: %v", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, ErrorLog: proxy.ErrorLog}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			_, _ = fmt.Fprintf(wafLog, "WAF server: %v\n", serveErr)
		}
	}()
	t.Cleanup(func() {
		context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(context); err != nil {
			t.Errorf("stop e2e WAF: %v", err)
		}
		if err := wafLog.Close(); err != nil {
			t.Errorf("close e2e WAF log: %v", err)
		}
	})
}

func (environment *testEnvironment) waitForServer(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(serverWaitTimeout)
	for time.Now().Before(deadline) {
		response, err := environment.httpClient.Get(environment.server + "/alive")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("Vaultwarden did not become ready")
}

func (environment *testEnvironment) seedVault(t *testing.T) {
	t.Helper()
	ca := filepath.Join(environment.state, "ca.pem")
	environment.run(t, nil, nil, "cargo", "run", "--quiet", "--manifest-path", filepath.Join(environment.root, "Cargo.toml"), "-p", "bwkp-e2e-register", "--", environment.server, sourceEmail, masterPassword, ca)
	environment.run(t, nil, nil, "cargo", "run", "--quiet", "--manifest-path", filepath.Join(environment.root, "Cargo.toml"), "-p", "bwkp-e2e-register", "--", environment.server, destinationEmail, masterPassword, ca)

	privateKey := filepath.Join(environment.state, "id_ed25519")
	environment.run(t, nil, nil, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "bwkp-e2e", "-f", privateKey)
	fingerprintOutput := environment.run(t, nil, nil, "ssh-keygen", "-lf", privateKey+".pub", "-E", "sha256")
	fingerprintFields := strings.Fields(fingerprintOutput)
	if len(fingerprintFields) < 2 {
		t.Fatalf("parse SSH fingerprint from %q", fingerprintOutput)
	}

	fixture := readJSON[map[string]any](t, filepath.Join(environment.root, "test", "e2e", "fixtures", "vault.json"))
	items, ok := fixture["items"].([]any)
	if !ok {
		t.Fatal("vault fixture has no items array")
	}
	for _, value := range items {
		item, itemOK := value.(map[string]any)
		if !itemOK || item["type"] != float64(5) {
			continue
		}
		sshKey, keyOK := item["sshKey"].(map[string]any)
		if !keyOK {
			t.Fatal("SSH fixture item has no sshKey object")
		}
		sshKey["privateKey"] = string(readFile(t, privateKey))
		sshKey["publicKey"] = strings.TrimSpace(string(readFile(t, privateKey+".pub")))
		sshKey["keyFingerprint"] = fingerprintFields[1]
	}
	vaultPath := filepath.Join(environment.state, "vault.json")
	writeJSON(t, vaultPath, fixture)

	environment.bw(t, environment.cliData, nil, "config", "server", environment.server)
	session := strings.TrimSpace(environment.bw(t, environment.cliData, nil, "login", "--raw", "--nointeraction", sourceEmail, masterPassword))
	environment.bw(t, environment.cliData, nil, "--session", session, "import", "bitwardenjson", vaultPath)
	environment.bw(t, environment.cliData, nil, "--session", session, "sync")

	itemID := firstItemID(t, environment.bw(t, environment.cliData, nil, "--session", session, "list", "items", "--search", "Attachment Item", "--raw"))
	environment.bw(t, environment.cliData, nil, "--session", session, "create", "attachment", "--file", filepath.Join(environment.root, "test", "e2e", "fixtures", "attachment.bin"), "--itemid", itemID)
	environment.bw(t, environment.cliData, nil, "--session", session, "create", "attachment", "--file", filepath.Join(environment.root, "test", "e2e", "fixtures", "attachment-second.bin"), "--itemid", itemID)
	archiveID := firstItemID(t, environment.bw(t, environment.cliData, nil, "--session", session, "list", "items", "--search", "Archived Item", "--raw"))
	deleteID := firstItemID(t, environment.bw(t, environment.cliData, nil, "--session", session, "list", "items", "--search", "Deleted Item", "--raw"))
	environment.bw(t, environment.cliData, nil, "--session", session, "archive", "item", archiveID)
	environment.bw(t, environment.cliData, nil, "--session", session, "delete", "item", deleteID)
	environment.bw(t, environment.cliData, nil, "logout")
}

func (environment *testEnvironment) enableTOTP(t *testing.T) {
	t.Helper()
	environment.compose(t, "stop", "vaultwarden")
	database := filepath.Join(environment.state, "vaultwarden", "db.sqlite3")
	writableDatabase := database + ".writable"
	writeFile(t, writableDatabase, readFile(t, database), 0o600)
	t.Cleanup(func() { _ = os.Remove(writableDatabase) })
	userID := strings.TrimSpace(environment.run(t, nil, nil, "sqlite3", writableDatabase, "select uuid from users where email='"+sourceEmail+"';"))
	factorID := randomUUID(t)
	statement := fmt.Sprintf("insert into twofactor(uuid,user_uuid,atype,enabled,data,last_used) values('%s','%s',0,1,'%s',0);", factorID, userID, totpSecret)
	environment.run(t, nil, nil, "sqlite3", writableDatabase, statement)
	if err := os.Rename(writableDatabase, database); err != nil {
		t.Fatalf("replace Vaultwarden database: %v", err)
	}
	environment.compose(t, "start", "vaultwarden")
	environment.waitForServer(t)
	environment.writeSecret(t, environment.totpFile, generateTOTP(t, totpSecret, time.Now()))
}

func (environment *testEnvironment) exportSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(environment.output, "vault.kdbx")
	environment.run(t, nil, nil, filepath.Join(environment.root, "dist", "bwkp"), "export", "--server", environment.server, "--email", sourceEmail, "--output", path, "--master-password-file", environment.masterFile, "--database-password-file", environment.databaseFile, "--totp-file", environment.totpFile, "--append-source", "--kdf-memory-kib", "8192", "--kdf-parallelism", "1", "--kdf-iterations", "10")
	return path
}

func (environment *testEnvironment) addUnsupportedEntry(t *testing.T, database string) {
	t.Helper()
	output, err := environment.command(t, strings.NewReader(databasePassword+"\n"), nil, "keepassxc-cli", "add", "-q", "--notes", "Unsupported KeePassXC data preserved as a note.", database, "Personal/Unfiled/Unsupported KeePassXC Entry")
	if err == nil {
		return
	}
	listing := environment.keepass(t, "ls", "-q", "-R", "-f", database)
	if !strings.Contains(listing, "Personal/Unfiled/Unsupported KeePassXC Entry") {
		t.Fatalf("add unsupported KeePassXC entry: %v\n%s", err, output)
	}
}

func (environment *testEnvironment) loginDestination(t *testing.T) string {
	t.Helper()
	environment.bw(t, environment.importData, nil, "config", "server", environment.server)
	return strings.TrimSpace(environment.bw(t, environment.importData, nil, "login", "--raw", "--nointeraction", destinationEmail, masterPassword))
}

func (environment *testEnvironment) importVault(t *testing.T, session, database, mode string) string {
	t.Helper()
	output := environment.run(t, nil, nil, filepath.Join(environment.root, "dist", "bwkp"), "import", "--server", environment.server, "--email", destinationEmail, "--input", database, "--master-password-file", environment.masterFile, "--database-password-file", environment.databaseFile, "--conflict", mode, "--no-progress")
	environment.bw(t, environment.importData, nil, "--session", session, "sync")
	return output
}

func (environment *testEnvironment) exportDestination(t *testing.T) string {
	t.Helper()
	path := filepath.Join(environment.output, "imported-vault.kdbx")
	environment.run(t, nil, nil, filepath.Join(environment.root, "dist", "bwkp"), "export", "--server", environment.server, "--email", destinationEmail, "--output", path, "--master-password-file", environment.masterFile, "--database-password-file", environment.databaseFile, "--no-progress", "--kdf-memory-kib", "8192", "--kdf-parallelism", "1", "--kdf-iterations", "10")
	return path
}

func (environment *testEnvironment) bw(t *testing.T, dataDirectory string, stdin io.Reader, arguments ...string) string {
	t.Helper()
	return environment.run(t, stdin, []string{"BITWARDENCLI_APPDATA_DIR=" + dataDirectory, "NODE_EXTRA_CA_CERTS=" + filepath.Join(environment.state, "ca.pem")}, "npx", append([]string{"-y", "@bitwarden/cli@" + bitwardenVersion}, arguments...)...)
}

func (environment *testEnvironment) logout(t *testing.T, dataDirectory string) {
	t.Helper()
	context, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(context, "npx", "-y", "@bitwarden/cli@"+bitwardenVersion, "logout")
	command.Dir = environment.root
	command.Env = mergedEnvironment([]string{
		"SSL_CERT_FILE=" + filepath.Join(environment.state, "ca.pem"),
		"NODE_EXTRA_CA_CERTS=" + filepath.Join(environment.state, "ca.pem"),
		"BITWARDENCLI_APPDATA_DIR=" + dataDirectory,
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Errorf("log out of Bitwarden CLI: %v\n%s", err, output)
	}
}

func (environment *testEnvironment) keepass(t *testing.T, arguments ...string) string {
	t.Helper()
	return environment.run(t, strings.NewReader(databasePassword+"\n"), nil, "keepassxc-cli", arguments...)
}

func (environment *testEnvironment) run(t *testing.T, stdin io.Reader, extraEnvironment []string, name string, arguments ...string) string {
	t.Helper()
	output, err := environment.command(t, stdin, extraEnvironment, name, arguments...)
	if err != nil {
		t.Fatalf("run %s: %v\n%s", name, err, output)
	}
	return output
}

func (environment *testEnvironment) command(t *testing.T, stdin io.Reader, extraEnvironment []string, name string, arguments ...string) (string, error) {
	t.Helper()
	context, cancel := context.WithTimeout(t.Context(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(context, name, arguments...)
	command.Dir = environment.root
	command.Env = mergedEnvironment(append([]string{"SSL_CERT_FILE=" + filepath.Join(environment.state, "ca.pem")}, extraEnvironment...))
	command.Stdin = stdin
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if context.Err() != nil {
		return stdout.String() + stderr.String(), fmt.Errorf("command timed out: %w", context.Err())
	}
	if err != nil {
		return stdout.String() + stderr.String(), err
	}
	return stdout.String(), nil
}

func (environment *testEnvironment) compose(t *testing.T, arguments ...string) {
	t.Helper()
	environment.run(t, nil, nil, "docker", append([]string{"compose", "-f", filepath.Join(environment.root, "test", "e2e", "compose.yml")}, arguments...)...)
}

func (environment *testEnvironment) composeCleanup(t *testing.T) {
	t.Helper()
	context, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(context, "docker", "compose", "-f", filepath.Join(environment.root, "test", "e2e", "compose.yml"), "down", "--volumes", "--remove-orphans")
	command.Dir = environment.root
	if output, err := command.CombinedOutput(); err != nil {
		t.Errorf("stop e2e services: %v\n%s", err, output)
	}
}

func (environment *testEnvironment) writeSecret(t *testing.T, path, value string) {
	t.Helper()
	writeFile(t, path, []byte(value+"\n"), 0o600)
}

func randomSerial(t *testing.T) *big.Int {
	t.Helper()
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		t.Fatalf("generate certificate serial: %v", err)
	}
	return serial
}

func randomUUID(t *testing.T) string {
	t.Helper()
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate UUID: %v", err)
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func generateTOTP(t *testing.T, encodedSecret string, now time.Time) string {
	t.Helper()
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encodedSecret))
	if err != nil {
		t.Fatalf("decode TOTP secret: %v", err)
	}
	defer clear(secret)
	counter := now.Unix() / 30
	if counter < 0 {
		t.Fatal("system time predates the Unix epoch")
	}
	message := make([]byte, 8)
	// #nosec G115 -- the negative case is rejected above.
	binary.BigEndian.PutUint64(message, uint64(counter))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(message)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000)
}

func firstItemID(t *testing.T, data string) string {
	t.Helper()
	items := decodeJSON[[]vaultItem](t, data)
	if len(items) == 0 {
		t.Fatal("Bitwarden CLI returned no matching items")
	}
	return items[0].ID
}

type vaultItem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Notes       string            `json:"notes"`
	Type        int               `json:"type"`
	FolderID    string            `json:"folderId"`
	Attachments []json.RawMessage `json:"attachments"`
}

type vaultFolder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func readJSON[T any](t *testing.T, path string) T {
	t.Helper()
	return decodeJSON[T](t, string(readFile(t, path)))
}

func decodeJSON[T any](t *testing.T, data string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(data), &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return value
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	writeFile(t, path, data, 0o600)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func itemNamed(items []vaultItem, name string) (vaultItem, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return vaultItem{}, false
}

func folderIDs(folders []vaultFolder) map[string]string {
	result := make(map[string]string, len(folders))
	for _, folder := range folders {
		result[folder.Name] = folder.ID
	}
	return result
}

func statusCode(t *testing.T, client *http.Client, method, target string) int {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, target, nil)
	if err != nil {
		t.Fatalf("create HTTP request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send HTTP request: %v", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

func mergedEnvironment(overrides []string) []string {
	if len(overrides) == 0 {
		return os.Environ()
	}
	names := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		name, _, _ := strings.Cut(override, "=")
		names[name] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, overridden := names[name]; !overridden {
			environment = append(environment, entry)
		}
	}
	return append(environment, overrides...)
}
