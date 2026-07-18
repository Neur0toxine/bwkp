package e2e

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportAndImport(t *testing.T) {
	environment := newTestEnvironment(t)

	if !t.Run("WAF rejects unofficial requests", func(t *testing.T) {
		attachmentStatus := statusCode(t, environment.httpClient, http.MethodGet, environment.server+"/attachments/test-cipher/test-file?token=test")
		if attachmentStatus != http.StatusUnauthorized {
			t.Errorf("attachment request status = %d, want %d", attachmentStatus, http.StatusUnauthorized)
		}
		mutationStatus := statusCode(t, environment.httpClient, http.MethodPost, environment.server+"/api/ciphers")
		if mutationStatus != http.StatusUnauthorized {
			t.Errorf("mutation request status = %d, want %d", mutationStatus, http.StatusUnauthorized)
		}
	}) {
		return
	}

	environment.seedVault(t)
	environment.enableTOTP(t)
	database := environment.exportSource(t)
	environment.addUnsupportedEntry(t, database)

	if !t.Run("exported database contains mapped vault data", func(t *testing.T) {
		listing := environment.keepass(t, "ls", "-q", "-R", "-f", database)
		for _, title := range []string{
			"Complex Login",
			"Secure Note",
			"Payment Card",
			"Full Identity",
			"SSH Key",
			"Attachment Item",
			"Archived Item",
			"Deleted Item",
			"Unsupported KeePassXC Entry",
		} {
			if !strings.Contains(listing, title) {
				t.Errorf("database listing does not contain %q", title)
			}
		}
		environment.keepass(t, "db-info", "-q", database)

		login := environment.keepass(t, "show", "-q", "--all", "--show-protected", database, "Personal/Engineering/Production/Complex Login")
		for _, field := range []string{
			"UserName: alice@example.test",
			"Password: correct horse battery staple",
			"Environment: production",
			"KP2A_URL_1: https://admin.example.test",
			"otp: otpauth://",
		} {
			if !strings.Contains(login, field) {
				t.Errorf("exported login does not contain %q", field)
			}
		}

		exportedAttachment := filepath.Join(environment.output, "attachment.bin")
		environment.keepass(t, "attachment-export", "-q", database, "Personal/Unfiled/Attachment Item", "attachment.bin", exportedAttachment)
		wantAttachment := readFile(t, filepath.Join(environment.root, "test", "e2e", "fixtures", "attachment.bin"))
		if gotAttachment := readFile(t, exportedAttachment); !bytes.Equal(gotAttachment, wantAttachment) {
			t.Error("exported attachment differs from source fixture")
		}
	}) {
		return
	}

	session := environment.loginDestination(t)
	t.Cleanup(func() { environment.logout(t, environment.importData) })

	if !t.Run("initial import creates items and folders", func(t *testing.T) {
		output := environment.importVault(t, session, database, "skip")
		if !strings.Contains(output, "9 created") {
			t.Errorf("initial import output does not report 9 created items:\n%s", output)
		}

		items := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--raw"))
		folders := folderIDs(decodeJSON[[]vaultFolder](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "folders", "--raw")))
		for title, folder := range map[string]string{
			"Complex Login":   "Engineering/Production",
			"Secure Note":     "Personal",
			"Attachment Item": "Unfiled",
		} {
			item, ok := itemNamed(items, title)
			if !ok {
				t.Errorf("imported items do not contain %q", title)
				continue
			}
			if item.FolderID != folders[folder] {
				t.Errorf("%q folder ID = %q, want folder %q ID %q", title, item.FolderID, folder, folders[folder])
			}
		}
	}) {
		return
	}

	if !t.Run("skip preserves existing items", func(t *testing.T) {
		output := environment.importVault(t, session, database, "skip")
		if !strings.Contains(output, "9 skipped") {
			t.Errorf("skip import output does not report 9 skipped items:\n%s", output)
		}
	}) {
		return
	}

	initialLogin := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--search", "Complex Login", "--raw"))
	if len(initialLogin) != 1 {
		t.Fatalf("Complex Login item count = %d, want 1", len(initialLogin))
	}
	initialLoginID := initialLogin[0].ID
	environment.keepass(t, "edit", "-q", "--notes", "Updated from KeePassXC", database, "Personal/Engineering/Production/Complex Login")

	if !t.Run("update changes items in place and preserves attachments", func(t *testing.T) {
		output := environment.importVault(t, session, database, "update")
		if !strings.Contains(output, "9 updated") {
			t.Errorf("update import output does not report 9 updated items:\n%s", output)
		}
		updated := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--search", "Complex Login", "--raw"))
		if len(updated) != 1 {
			t.Fatalf("updated Complex Login item count = %d, want 1", len(updated))
		}
		if updated[0].ID != initialLoginID {
			t.Errorf("updated item ID = %q, want original ID %q", updated[0].ID, initialLoginID)
		}
		if updated[0].Notes != "Updated from KeePassXC" {
			t.Errorf("updated notes = %q, want %q", updated[0].Notes, "Updated from KeePassXC")
		}

		attachments := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--search", "Attachment Item", "--raw"))
		if len(attachments) != 1 || len(attachments[0].Attachments) != 2 {
			t.Errorf("imported Attachment Item = %#v, want one item with two attachments", attachments)
		}
		importedDatabase := environment.exportDestination(t)
		importedAttachment := filepath.Join(environment.output, "imported-attachment.bin")
		environment.keepass(t, "attachment-export", "-q", importedDatabase, "Personal/Unfiled/Attachment Item", "attachment.bin", importedAttachment)
		wantAttachment := readFile(t, filepath.Join(environment.root, "test", "e2e", "fixtures", "attachment.bin"))
		if gotAttachment := readFile(t, importedAttachment); !bytes.Equal(gotAttachment, wantAttachment) {
			t.Error("round-tripped attachment differs from source fixture")
		}

		unsupported := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--search", "Unsupported KeePassXC Entry", "--raw"))
		if len(unsupported) != 1 || unsupported[0].Type != 2 || !strings.Contains(unsupported[0].Notes, "preserved as a note") {
			t.Errorf("unsupported entry = %#v, want one secure note preserving the original note", unsupported)
		}
	}) {
		return
	}

	if !t.Run("delete replaces items and moves originals to trash", func(t *testing.T) {
		output := environment.importVault(t, session, database, "delete")
		if !strings.Contains(output, "9 replaced") {
			t.Errorf("delete import output does not report 9 replaced items:\n%s", output)
		}
		replacements := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--search", "Complex Login", "--raw"))
		if len(replacements) != 1 {
			t.Fatalf("replacement Complex Login item count = %d, want 1", len(replacements))
		}
		replacementLoginID := replacements[0].ID
		if replacementLoginID == initialLoginID {
			t.Error("delete conflict mode kept the original destination item ID")
		}
		trash := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--trash", "--search", "Complex Login", "--raw"))
		foundOriginal := false
		for _, item := range trash {
			foundOriginal = foundOriginal || item.ID == initialLoginID
		}
		if !foundOriginal {
			t.Errorf("trash does not contain original item ID %q", initialLoginID)
		}
	}) {
		return
	}

	t.Run("duplicate creates an additional item", func(t *testing.T) {
		before := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--search", "Complex Login", "--raw"))
		output := environment.importVault(t, session, database, "duplicate")
		if !strings.Contains(output, "9 duplicated") {
			t.Errorf("duplicate import output does not report 9 duplicated items:\n%s", output)
		}
		after := decodeJSON[[]vaultItem](t, environment.bw(t, environment.importData, nil, "--session", session, "list", "items", "--search", "Complex Login", "--raw"))
		if len(after) != len(before)+1 {
			t.Errorf("Complex Login item count after duplicate = %d, want %d", len(after), len(before)+1)
		}
	})
}
