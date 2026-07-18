//go:build native && cgo

package kpdb_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Neur0toxine/bwkp/internal/native"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
	"github.com/Neur0toxine/bwkp/pkg/kpdb"
)

func TestNativeWriterProducesKeePassXCCompatibleDatabase(t *testing.T) {
	if got := native.KeePassXCVersion(); got != "2.7.12" {
		t.Fatalf("unexpected linked KeePassXC version %q", got)
	}
	target := filepath.Join(t.TempDir(), "test.kdbx")
	options := kpdb.DefaultOptions()
	options.Iterations, options.TargetTime = 10, 0
	options.MemoryKiB, options.Parallelism = 8*1024, 1
	database := kp.Database{Name: "Test", Root: kp.Group{
		Name: "Test", Entries: []kp.Entry{{
			Title: "Entry", Username: "alice", Password: kp.ProtectedString{Value: "secret"},
			Fields:      map[string]kp.Value{"otp": {Value: "otpauth://totp/Test?secret=JBSWY3DPEHPK3PXP", Protected: true}},
			Attachments: []kp.Attachment{{Name: "hello.txt", Content: []byte("hello")}},
		}},
	}}
	if err := kpdb.NewNativeWriter().WriteFile(t.Context(), target, database, kpdb.Credentials{Password: []byte("password")}, options, false); err != nil {
		t.Fatal(err)
	}
	commandPath, err := exec.LookPath("keepassxc-cli")
	if err != nil {
		t.Skip("keepassxc-cli is not installed")
	}
	command := exec.CommandContext(t.Context(), commandPath, "db-info", "-q", target)
	command.Stdin = strings.NewReader("password\n")
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		t.Fatalf("KeePassXC could not open output: %v\n%s", err, output.String())
	}
}

func TestNativeReaderRoundTripsKeePassXCData(t *testing.T) {
	target := filepath.Join(t.TempDir(), "read.kdbx")
	options := kpdb.DefaultOptions()
	options.Iterations, options.TargetTime = 10, 0
	options.MemoryKiB, options.Parallelism = 8*1024, 1
	database := kp.Database{Name: "Read Test", Root: kp.Group{Name: "Root", Groups: []kp.Group{{Name: "Folder", Entries: []kp.Entry{{
		Title: "Entry", Username: "alice", Password: kp.ProtectedString{Value: "secret"},
		Fields:      map[string]kp.Value{"Hidden": {Value: "value", Protected: true}},
		Attachments: []kp.Attachment{{Name: "hello.bin", Content: []byte{0, 1, 2}}},
	}}}}}}
	credentials := kpdb.Credentials{Password: []byte("password")}
	if err := kpdb.NewNativeWriter().WriteFile(t.Context(), target, database, credentials, options, false); err != nil {
		t.Fatal(err)
	}
	read, err := kpdb.NewNativeReader().ReadFile(t.Context(), target, credentials)
	if err != nil {
		t.Fatal(err)
	}
	entry := read.Root.Groups[0].Entries[0]
	if entry.UUID == "" || entry.Username != "alice" || entry.Password.Value != "secret" || !entry.Fields["Hidden"].Protected || !bytes.Equal(entry.Attachments[0].Content, []byte{0, 1, 2}) {
		t.Fatalf("entry = %+v", entry)
	}
}
