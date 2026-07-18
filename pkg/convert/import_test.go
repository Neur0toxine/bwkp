package convert_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Neur0toxine/bwkp/pkg/convert"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
)

func TestImportConvertsLoginAndStripsExportWrappers(t *testing.T) {
	privateKey := base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3})
	entry := kp.Entry{
		UUID: "entry", Title: "Example", Username: "alice", Password: kp.ProtectedString{Value: "secret"}, URL: "https://example.test",
		Fields: map[string]kp.Value{
			"otp":                          {Value: "otpauth://totp/Test?secret=AAAA", Protected: true},
			"KP2A_URL_1":                   {Value: "https://admin.example.test"},
			"KPEX_PASSKEY_CREDENTIAL_ID":   {Value: "AQI", Protected: true},
			"KPEX_PASSKEY_PRIVATE_KEY_PEM": {Value: "-----BEGIN PRIVATE KEY-----\n" + base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) + "\n-----END PRIVATE KEY-----\n", Protected: true},
			"KPEX_PASSKEY_RELYING_PARTY":   {Value: "example.test"},
			"KPEX_PASSKEY_USERNAME":        {Value: "alice"},
			"KPEX_PASSKEY_USER_HANDLE":     {Value: "handle", Protected: true},
			"Custom":                       {Value: privateKey, Protected: true},
		},
	}
	database := kp.Database{Root: kp.Group{Name: "Bitwarden Export", Groups: []kp.Group{{
		Name: "Trash", Groups: []kp.Group{{Name: "Personal", Groups: []kp.Group{{Name: "Work", Entries: []kp.Entry{entry}}}}},
	}}}}

	entries, report, err := convert.NewKDBXConverter(convert.ImportOptions{}).Convert(database)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || report.Entries != 1 {
		t.Fatalf("entries=%+v report=%+v", entries, report)
	}
	converted := entries[0]
	if converted.Folder != "Work" || !converted.Trashed || converted.Item.Type != bw.ItemTypeLogin {
		t.Fatalf("entry = %+v", converted)
	}
	if converted.Item.Login == nil || len(converted.Item.Login.URIs) != 2 || len(converted.Item.Login.FIDO2Credentials) != 1 {
		t.Fatalf("login = %+v", converted.Item.Login)
	}
	custom := findBWField(converted.Item.Fields, "Custom")
	if custom == nil || custom.Type != 1 {
		t.Fatalf("fields = %+v", converted.Item.Fields)
	}
}

func TestImportUsesCurrentEntryOverSourceAndAppendsDeterministicSource(t *testing.T) {
	source := `{"id":"old-id","type":"card","name":"Old","card":{"number":"1111"}}`
	database := kp.Database{Root: kp.Group{Name: "Root", Entries: []kp.Entry{{
		UUID: "uuid", Title: "Edited", Fields: map[string]kp.Value{
			"BW.SourceJSON": {Value: source, Protected: true}, "Card Number": {Value: "2222", Protected: true},
		}, Attachments: []kp.Attachment{{Name: "file.bin", Content: []byte("content")}},
	}}}}
	converter := convert.NewKDBXConverter(convert.ImportOptions{AppendSource: true})
	first, _, err := converter.Convert(database)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := converter.Convert(database)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Item.Name != "Edited" || first[0].Item.Card == nil || first[0].Item.Card.Number != "2222" || first[0].SourceItemID != "old-id" {
		t.Fatalf("entry = %+v", first[0])
	}
	a := findBWField(first[0].Item.Fields, "KP.SourceJSON")
	b := findBWField(second[0].Item.Fields, "KP.SourceJSON")
	if a == nil || b == nil || a.Value != b.Value || strings.Contains(a.Value, base64.StdEncoding.EncodeToString([]byte("content"))) || !strings.Contains(a.Value, `"sha256"`) {
		t.Fatalf("source fields = %#v %#v", a, b)
	}
}

func TestImportSkipsTemplatesAndFallsBackUnknownTypeToNote(t *testing.T) {
	database := kp.Database{Root: kp.Group{Name: "Root", Groups: []kp.Group{
		{Name: "Templates", Templates: true, Entries: []kp.Entry{{Title: "Template"}}},
		{Name: "Data", Entries: []kp.Entry{{UUID: "unknown", Title: "Future", Username: "user", Fields: map[string]kp.Value{
			"BW.ItemType": {Value: "future", Protected: true}, "Secret": {Value: "value", Protected: true},
		}}}},
	}}}
	entries, report, err := convert.NewKDBXConverter(convert.ImportOptions{}).Convert(database)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].FallbackNote || entries[0].Item.Type != bw.ItemTypeSecureNote || !strings.Contains(entries[0].Item.Notes, "Username: user") || report.Fallbacks != 1 || len(report.Warnings) != 1 {
		t.Fatalf("entries=%+v report=%+v", entries, report)
	}
}

func TestImportRestoresBitwardenFieldMetadata(t *testing.T) {
	database := kp.Database{Root: kp.Group{Name: "Root", Entries: []kp.Entry{{
		Title: "Metadata", URL: "https://example.test", Fields: map[string]kp.Value{
			"BW.Reprompt":              {Value: "true", Protected: true},
			"BW.URIMatch.1":            {Value: "2", Protected: true},
			"Account":                  {Value: "value"},
			"BW.FieldType.Account":     {Value: "2", Protected: true},
			"BW.LinkedField.Account":   {Value: "7", Protected: true},
			"BW.FutureMetadata":        {Value: "preserve", Protected: true},
			"Bitwarden ordinary field": {Value: "ordinary"},
		},
	}}}}
	entries, _, err := convert.NewKDBXConverter(convert.ImportOptions{}).Convert(database)
	if err != nil {
		t.Fatal(err)
	}
	item := entries[0].Item
	if !item.Reprompt || item.Login == nil || item.Login.URIs[0].Match == nil || *item.Login.URIs[0].Match != 2 {
		t.Fatalf("item = %+v", item)
	}
	account := findBWField(item.Fields, "Account")
	future := findBWField(item.Fields, "BW.FutureMetadata")
	ordinary := findBWField(item.Fields, "Bitwarden ordinary field")
	if account == nil || account.Type != 2 || account.Linked == nil || *account.Linked != 7 || future == nil || future.Type != 1 || ordinary == nil {
		t.Fatalf("fields = %+v", item.Fields)
	}
}

func TestImportConvertsEveryDestinationItemType(t *testing.T) {
	entries := []kp.Entry{
		{Title: "Login", Username: "alice"},
		{Title: "Note", Notes: "note"},
		{Title: "Card", Fields: map[string]kp.Value{"Card Number": {Value: "4111"}}},
		{Title: "Identity", Fields: map[string]kp.Value{"First Name": {Value: "Alice"}}},
		{Title: "SSH", Attachments: []kp.Attachment{{Name: "id_ssh", Content: []byte("private")}}},
		{Title: "Bank", Fields: map[string]kp.Value{"BW.ItemType": {Value: "bankAccount"}, "Bitwarden bankName": {Value: `"Example"`}}},
		{Title: "License", Fields: map[string]kp.Value{"BW.ItemType": {Value: "driversLicense"}, "Bitwarden licenseNumber": {Value: `"D123"`}}},
		{Title: "Passport", Fields: map[string]kp.Value{"BW.ItemType": {Value: "passport"}, "Bitwarden passportNumber": {Value: `"P123"`}}},
	}
	database := kp.Database{Root: kp.Group{Name: "Root", Entries: entries}}
	converted, _, err := convert.NewKDBXConverter(convert.ImportOptions{}).Convert(database)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bw.ItemType{"Login": bw.ItemTypeLogin, "Note": bw.ItemTypeSecureNote, "Card": bw.ItemTypeCard, "Identity": bw.ItemTypeIdentity, "SSH": bw.ItemTypeSSHKey, "Bank": bw.ItemTypeBankAccount, "License": bw.ItemTypeDriversLicense, "Passport": bw.ItemTypePassport}
	if len(converted) != len(want) {
		t.Fatalf("converted %d entries, want %d", len(converted), len(want))
	}
	byName := make(map[string]bw.Item, len(converted))
	for _, entry := range converted {
		byName[entry.Item.Name] = entry.Item
	}
	for name, itemType := range want {
		if byName[name].Type != itemType {
			t.Errorf("entry %q type = %q, want %q", name, byName[name].Type, itemType)
		}
	}
	if byName["Bank"].Data["bankName"] != "Example" || byName["License"].Data["licenseNumber"] != "D123" || byName["Passport"].Data["passportNumber"] != "P123" {
		t.Fatalf("structured data was not restored: %+v %+v %+v", byName["Bank"].Data, byName["License"].Data, byName["Passport"].Data)
	}
}

func findBWField(fields []bw.Field, name string) *bw.Field {
	for _, field := range fields {
		if field.Name == name {
			return new(field)
		}
	}
	return nil
}
