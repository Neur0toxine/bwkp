package convert_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/Neur0toxine/bwkp/pkg/convert"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
)

func TestConvertLoginOmitsConvertedSourceMetadataByDefault(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	match := 1
	vault := bw.Vault{
		Folders: []bw.Folder{{ID: "folder", Name: "Work/Production"}},
		Items: []bw.Item{{
			ID: "item", FolderID: "folder", Type: bw.ItemTypeLogin, Name: "Example",
			CreationDate: now, RevisionDate: now, Favorite: true,
			Login: &bw.Login{
				Username: "alice", Password: "secret", TOTP: "JBSWY3DPEHPK3PXP",
				URIs: []bw.URI{{URI: "https://example.test", Match: &match}, {URI: "https://admin.example.test"}},
			},
			Fields: []bw.Field{{Name: "PIN", Value: "1234", Type: 1}},
		}},
	}

	db, report, err := convert.New().Convert(vault)
	if err != nil {
		t.Fatal(err)
	}
	if report.Items != 1 || report.Entries != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	entry := mustEntry(t, db.Root, "Personal", "Work", "Production")
	if entry.Username != "alice" || entry.Password.Value != "secret" || entry.URL != "https://example.test" {
		t.Fatalf("login fields not mapped: %+v", entry)
	}
	if !entry.Fields["otp"].Protected || !strings.HasPrefix(entry.Fields["otp"].Value, "otpauth://") {
		t.Fatalf("TOTP not mapped: %+v", entry.Fields["otp"])
	}
	if _, exists := entry.Fields["BW.SourceJSON"]; exists {
		t.Fatal("source JSON should require AppendSource")
	}
	if _, exists := entry.Fields["BW.ItemID"]; exists {
		t.Fatal("source identity should require AppendSource")
	}
	if matchField := entry.Fields["BW.URIMatch.1"]; matchField.Value != "1" || !matchField.Protected {
		t.Fatalf("unconverted URI match metadata = %+v", matchField)
	}
}

func TestConvertAppendsCompleteSourceMetadataWhenRequested(t *testing.T) {
	vault := bw.Vault{Items: []bw.Item{{
		ID: "item", FolderID: "folder", CollectionIDs: []string{"collection"},
		Type: bw.ItemTypeSecureNote, Name: "Source",
	}}}

	db, _, err := convert.NewWithOptions(convert.Options{AppendSource: true}).Convert(vault)
	if err != nil {
		t.Fatal(err)
	}
	entry := db.Root.Groups[0].Groups[0].Entries[0]
	if !entry.Fields["BW.SourceJSON"].Protected || !strings.Contains(entry.Fields["BW.SourceJSON"].Value, `"id":"item"`) {
		t.Fatal("protected source JSON missing")
	}
	if entry.Fields["BW.ItemID"].Value != "item" || entry.Fields["BW.FolderID"].Value != "folder" {
		t.Fatalf("source identity metadata missing: %+v", entry.Fields)
	}
	if entry.Fields["BW.CollectionIDs"].Value != `["collection"]` {
		t.Fatalf("collection metadata = %q", entry.Fields["BW.CollectionIDs"].Value)
	}
}

func TestConvertReportsItemProgress(t *testing.T) {
	vault := bw.Vault{Items: []bw.Item{
		{ID: "b", Type: bw.ItemTypeSecureNote, Name: "Second"},
		{ID: "a", Type: bw.ItemTypeSecureNote, Name: "First"},
	}}
	var updates [][2]int

	_, _, err := convert.New().ConvertWithProgress(vault, func(completed, total int) {
		updates = append(updates, [2]int{completed, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]int{{1, 2}, {2, 2}}
	if len(updates) != len(want) || updates[0] != want[0] || updates[1] != want[1] {
		t.Fatalf("progress = %v, want %v", updates, want)
	}
}

func TestConvertSplitsMultiplePasskeys(t *testing.T) {
	key := base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4})
	vault := bw.Vault{Items: []bw.Item{{
		ID: "item", Type: bw.ItemTypeLogin, Name: "Passkeys", Login: &bw.Login{
			FIDO2Credentials: []bw.FIDO2Credential{
				{CredentialID: "0102", KeyValue: key, RPID: "example.test", RPName: "Example", UserName: "one", UserDisplayName: "One User", Counter: "4"},
				{CredentialID: "0304", KeyValue: key, RPID: "example.test", UserName: "two"},
			},
		},
	}}}
	db, report, err := convert.New().Convert(vault)
	if err != nil {
		t.Fatal(err)
	}
	if report.Entries != 2 || report.Passkeys != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	entries := db.Root.Groups[0].Groups[0].Entries
	if got := entries[0].Fields["KPEX_PASSKEY_CREDENTIAL_ID"].Value; got != "AQI" {
		t.Fatalf("credential ID = %q", got)
	}
	if !strings.Contains(entries[0].Fields["KPEX_PASSKEY_PRIVATE_KEY_PEM"].Value, "BEGIN PRIVATE KEY") {
		t.Fatal("private key was not converted to PEM")
	}
	if entries[0].Fields["BW.Passkey.RPName"].Value != "Example" || entries[0].Fields["BW.Passkey.Counter"].Value != "4" {
		t.Fatalf("unconverted passkey metadata missing: %+v", entries[0].Fields)
	}
}

func TestConvertOrganizationTrashUsesCollection(t *testing.T) {
	deleted := time.Now()
	vault := bw.Vault{
		Organizations: []bw.Organization{{ID: "org", Name: "Acme"}},
		Collections:   []bw.Collection{{ID: "b", OrganizationID: "org", Name: "Zed"}, {ID: "a", OrganizationID: "org", Name: "Apps/Prod"}},
		Items:         []bw.Item{{ID: "x", OrganizationID: "org", CollectionIDs: []string{"b", "a"}, DeletedDate: &deleted, Type: bw.ItemTypeSecureNote, Name: "Deleted"}},
	}
	db, _, err := convert.New().Convert(vault)
	if err != nil {
		t.Fatal(err)
	}
	entry := mustEntry(t, db.Root, "Trash", "Organizations", "Acme", "Apps", "Prod")
	if entry.Title != "Deleted" {
		t.Fatalf("title = %q", entry.Title)
	}
	if entry.Fields["BW.CollectionIDs"].Value != `["b","a"]` || entry.Fields["BW.DeletedDate"].Value == "" {
		t.Fatalf("unconverted membership metadata missing: %+v", entry.Fields)
	}
}

func TestConvertRejectsUnknownType(t *testing.T) {
	_, _, err := convert.New().Convert(bw.Vault{Items: []bw.Item{{ID: "x", Type: "future", Name: "Future"}}})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v", err)
	}
}

func TestConvertMapsSpecializedItemsAndDuplicateData(t *testing.T) {
	linked := 7
	vault := bw.Vault{Items: []bw.Item{
		{ID: "card", Type: bw.ItemTypeCard, Name: "Card", Card: &bw.Card{CardholderName: "Alice", Number: "4111", Code: "123"}},
		{ID: "identity", Type: bw.ItemTypeIdentity, Name: "Identity", Identity: &bw.Identity{FirstName: "Alice", Email: "alice@example.test"}},
		{ID: "ssh", Type: bw.ItemTypeSSHKey, Name: "SSH", SSHKey: &bw.SSHKey{PrivateKey: "PRIVATE", PublicKey: "PUBLIC", Fingerprint: "SHA256:test"}, Attachments: []bw.Attachment{{FileName: "id_ssh", Content: []byte("collision")}}},
		{ID: "bank", Type: bw.ItemTypeBankAccount, Name: "Bank", Reprompt: true, Data: map[string]any{"accountNumber": "123"}, Fields: []bw.Field{{Name: "duplicate", Value: "one"}, {Name: "duplicate", Value: "two", Type: 1, Linked: &linked}, {Value: "unnamed", Type: 2}}},
	}}
	db, report, err := convert.New().Convert(vault)
	if err != nil {
		t.Fatal(err)
	}
	if report.Items != 4 || report.Attachments != 3 {
		t.Fatalf("report = %+v", report)
	}
	entries := db.Root.Groups[0].Groups[0].Entries
	byTitle := make(map[string]kp.Entry, len(entries))
	for _, entry := range entries {
		byTitle[entry.Title] = entry
	}
	if !byTitle["Card"].Fields["Card Number"].Protected {
		t.Fatal("card number is not protected")
	}
	if byTitle["Identity"].Fields["Email"].Value != "alice@example.test" {
		t.Fatal("identity email missing")
	}
	if len(byTitle["SSH"].Attachments) != 3 || byTitle["SSH"].Attachments[2].Name != "id_ssh (2)" {
		t.Fatalf("SSH attachments = %+v", byTitle["SSH"].Attachments)
	}
	if byTitle["Bank"].Fields["duplicate (2)"].Value != "two" || byTitle["Bank"].Fields["Bitwarden Field 3"].Value != "unnamed" {
		t.Fatal("custom fields not preserved")
	}
	if byTitle["Bank"].Fields["BW.Reprompt"].Value != "true" || byTitle["Bank"].Fields["BW.LinkedField.duplicate (2)"].Value != "7" || byTitle["Bank"].Fields["BW.FieldType.Bitwarden Field 3"].Value != "2" {
		t.Fatalf("unconverted source semantics missing: %+v", byTitle["Bank"].Fields)
	}
}

func TestConvertHistoryAndExistingTOTP(t *testing.T) {
	used := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	vault := bw.Vault{Items: []bw.Item{{
		ID: "login", Type: bw.ItemTypeLogin, Name: "History",
		Login:           &bw.Login{Password: "new", TOTP: "otpauth://totp/Existing?secret=AAAA"},
		PasswordHistory: []bw.PasswordHistory{{Password: "old", LastUsedDate: used}},
	}}}
	db, _, err := convert.New().Convert(vault)
	if err != nil {
		t.Fatal(err)
	}
	entry := db.Root.Groups[0].Groups[0].Entries[0]
	if len(entry.History) != 1 || entry.History[0].Password.Value != "old" {
		t.Fatal("password history missing")
	}
	if entry.Fields["otp"].Value != "otpauth://totp/Existing?secret=AAAA" {
		t.Fatal("existing TOTP URI changed")
	}
}

func TestConvertRejectsMalformedKnownPayloads(t *testing.T) {
	tests := []bw.Item{
		{ID: "login", Type: bw.ItemTypeLogin}, {ID: "card", Type: bw.ItemTypeCard},
		{ID: "identity", Type: bw.ItemTypeIdentity}, {ID: "ssh", Type: bw.ItemTypeSSHKey},
		{ID: "passkey", Type: bw.ItemTypeLogin, Login: &bw.Login{FIDO2Credentials: []bw.FIDO2Credential{{CredentialID: "not/base64!", KeyValue: "bad!"}}}},
	}
	for _, item := range tests {
		if _, _, err := convert.New().Convert(bw.Vault{Items: []bw.Item{item}}); err == nil {
			t.Fatalf("item %s should fail", item.ID)
		}
	}
}

func mustEntry(t *testing.T, root kp.Group, path ...string) kp.Entry {
	t.Helper()
	group := root
	for _, name := range path {
		found := false
		for _, candidate := range group.Groups {
			if candidate.Name == name {
				group, found = candidate, true
				break
			}
		}
		if !found {
			t.Fatalf("group %q missing below %q", name, group.Name)
		}
	}
	if len(group.Entries) != 1 {
		t.Fatalf("entries in %q = %d", group.Name, len(group.Entries))
	}
	return group.Entries[0]
}
