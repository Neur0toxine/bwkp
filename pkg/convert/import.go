package convert

import (
	"cmp"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
)

type ImportOptions struct {
	AppendSource bool
	AllowLossy   bool
}

type ImportEntry struct {
	SourceUUID   string
	SourceItemID string
	Folder       string
	Item         bw.Item
	Trashed      bool
	Archived     bool
	FallbackNote bool
}

type ImportReport struct {
	Entries     int
	Attachments int
	Fallbacks   int
	Warnings    []ImportWarning
}

type ImportWarning struct {
	EntryUUID string
	Title     string
	Message   string
}

type KDBXConverter struct {
	options ImportOptions
}

func NewKDBXConverter(options ImportOptions) *KDBXConverter {
	return &KDBXConverter{options: options}
}

func (c *KDBXConverter) Convert(database kp.Database) ([]ImportEntry, ImportReport, error) {
	var entries []ImportEntry
	report := ImportReport{}
	if err := c.walkGroup(database.Root, nil, false, &entries, &report); err != nil {
		return nil, ImportReport{}, err
	}
	slices.SortFunc(entries, func(a, b ImportEntry) int {
		return cmp.Or(cmp.Compare(a.Folder, b.Folder), cmp.Compare(a.Item.Name, b.Item.Name), cmp.Compare(a.SourceUUID, b.SourceUUID))
	})
	return entries, report, nil
}

func (c *KDBXConverter) walkGroup(group kp.Group, parent []string, recycled bool, output *[]ImportEntry, report *ImportReport) error {
	if group.Templates {
		return nil
	}
	path := parent
	if len(parent) > 0 {
		path = append(slices.Clone(parent), group.Name)
	} else {
		path = []string{group.Name}
	}
	recycled = recycled || group.RecycleBin
	for _, entry := range group.Entries {
		converted, warning, err := c.convertEntry(entry, path[1:], recycled || entry.Recycled)
		if err != nil {
			if !c.options.AllowLossy {
				return fmt.Errorf("convert KDBX entry %q (%s): %w", entry.Title, entry.UUID, err)
			}
			report.Warnings = append(report.Warnings, ImportWarning{EntryUUID: entry.UUID, Title: entry.Title, Message: err.Error()})
			continue
		}
		if warning != "" {
			report.Warnings = append(report.Warnings, ImportWarning{EntryUUID: entry.UUID, Title: entry.Title, Message: warning})
		}
		*output = append(*output, converted)
		report.Entries++
		report.Attachments += len(converted.Item.Attachments)
		if converted.FallbackNote {
			report.Fallbacks++
		}
	}
	for _, child := range group.Groups {
		if err := c.walkGroup(child, path, recycled, output, report); err != nil {
			return err
		}
	}
	return nil
}

func (c *KDBXConverter) convertEntry(entry kp.Entry, rawPath []string, recycled bool) (ImportEntry, string, error) {
	path, trashed := importPath(rawPath)
	trashed = trashed || recycled
	folder := strings.Join(path, "/")
	source, sourceID, sourceType, sourceWarning := sourceItem(entry)
	item := source
	item.ID = ""
	item.OrganizationID = ""
	item.FolderID = ""
	item.CollectionIDs = nil
	item.Name = cmp.Or(entry.Title, "Untitled")
	item.Notes = entry.Notes
	item.CreationDate = entry.Created
	item.RevisionDate = entry.Modified
	item.DeletedDate = nil
	item.ArchivedDate = nil
	item.Attachments = importAttachments(entry.Attachments)
	item.Favorite = slices.Contains(entry.Tags, "Favorite")

	typeHint := sourceType
	if typeHint == "" {
		typeHint = inferItemType(entry)
	}
	fallback := false
	switch typeHint {
	case bw.ItemTypeLogin:
		item.Type = bw.ItemTypeLogin
		item.Login = importLogin(entry, item.Login)
		item.Card, item.Identity, item.SSHKey, item.Data = nil, nil, nil, nil
	case bw.ItemTypeCard:
		item.Type = bw.ItemTypeCard
		item.Card = importCard(entry, item.Card)
		item.Login, item.Identity, item.SSHKey, item.Data = nil, nil, nil, nil
	case bw.ItemTypeIdentity:
		item.Type = bw.ItemTypeIdentity
		item.Identity = importIdentity(entry, item.Identity)
		item.Login, item.Card, item.SSHKey, item.Data = nil, nil, nil, nil
	case bw.ItemTypeSSHKey:
		if key, ok := importSSHKey(entry, item.SSHKey); ok {
			item.Type, item.SSHKey = bw.ItemTypeSSHKey, key
			item.Login, item.Card, item.Identity, item.Data = nil, nil, nil, nil
			item.Attachments = removeSSHControlAttachments(item.Attachments)
		} else {
			fallback = true
		}
	case bw.ItemTypeBankAccount, bw.ItemTypeDriversLicense, bw.ItemTypePassport:
		item.Type = typeHint
		item.Data = importStructuredData(entry, item.Data)
		item.Login, item.Card, item.Identity, item.SSHKey = nil, nil, nil, nil
	case bw.ItemTypeSecureNote, "":
		item.Type = bw.ItemTypeSecureNote
		item.Login, item.Card, item.Identity, item.SSHKey, item.Data = nil, nil, nil, nil, nil
	default:
		fallback = true
	}
	if fallback {
		item = fallbackNote(entry, item)
		sourceWarning = fmt.Sprintf("unsupported KDBX item type %q imported as a secure note", typeHint)
	}
	if value := fieldValue(entry, "BW.Reprompt"); value != "" {
		if reprompt, err := strconv.ParseBool(value); err == nil {
			item.Reprompt = reprompt
		}
	}

	consumed := consumedFields(item.Type)
	if _, err := strconv.ParseBool(fieldValue(entry, "BW.Reprompt")); err == nil && fieldValue(entry, "BW.Reprompt") != "" {
		consumed["BW.Reprompt"] = struct{}{}
	}
	if item.Login != nil {
		for index, uri := range item.Login.URIs {
			if uri.Match != nil {
				consumed[fmt.Sprintf("BW.URIMatch.%d", index+1)] = struct{}{}
			}
		}
		if len(item.Login.FIDO2Credentials) > 0 {
			for _, name := range passkeyFieldNames() {
				consumed[name] = struct{}{}
			}
			if !item.Login.FIDO2Credentials[0].CreationDate.IsZero() {
				consumed["BW.Passkey.CreationDate"] = struct{}{}
			}
		}
	}
	if item.Type == bw.ItemTypeBankAccount || item.Type == bw.ItemTypeDriversLicense || item.Type == bw.ItemTypePassport {
		for name := range entry.Fields {
			if strings.HasPrefix(name, "Bitwarden ") {
				consumed[name] = struct{}{}
			}
		}
	}
	item.Fields = importFields(entry, consumed)
	addPreservationFields(&item, entry)
	if c.options.AppendSource {
		raw, err := sourceJSON(entry, folder)
		if err != nil {
			return ImportEntry{}, "", fmt.Errorf("encode KDBX source: %w", err)
		}
		item.Fields = append(item.Fields, bw.Field{Name: "KP.SourceJSON", Value: string(raw), Type: 1})
	}
	archived := slices.Contains(entry.Tags, "Archived") || fieldValue(entry, "BW.ArchivedDate") != ""
	trashed = trashed || fieldValue(entry, "BW.DeletedDate") != ""
	return ImportEntry{
		SourceUUID: entry.UUID, SourceItemID: sourceID, Folder: folder, Item: item,
		Trashed: trashed, Archived: archived, FallbackNote: fallback,
	}, sourceWarning, nil
}

func importPath(path []string) ([]string, bool) {
	path = slices.Clone(path)
	trashed := false
	if len(path) > 0 && path[0] == "Trash" {
		path, trashed = path[1:], true
	}
	if len(path) > 0 && path[0] == "Personal" {
		path = path[1:]
	}
	return path, trashed
}

func sourceItem(entry kp.Entry) (bw.Item, string, bw.ItemType, string) {
	var item bw.Item
	if raw := fieldValue(entry, "BW.SourceJSON"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return bw.Item{}, fieldValue(entry, "BW.ItemID"), bw.ItemType(fieldValue(entry, "BW.ItemType")), "invalid BW.SourceJSON ignored: " + err.Error()
		}
	}
	id := cmp.Or(fieldValue(entry, "BW.ItemID"), item.ID)
	typeHint := bw.ItemType(cmp.Or(fieldValue(entry, "BW.ItemType"), string(item.Type)))
	return item, id, typeHint, ""
}

func inferItemType(entry kp.Entry) bw.ItemType {
	if _, ok := importSSHKey(entry, nil); ok {
		return bw.ItemTypeSSHKey
	}
	if hasAnyField(entry, "Cardholder Name", "Card Brand", "Card Number", "Expiry Month", "Expiry Year", "CVV") {
		return bw.ItemTypeCard
	}
	if hasAnyField(entry, "Identity Title", "First Name", "Last Name", "Address 1", "Email", "SSN", "Passport Number", "License Number") {
		return bw.ItemTypeIdentity
	}
	if entry.Username != "" || entry.Password.Value != "" || entry.URL != "" || hasAnyField(entry, "otp", "TOTP Seed", "TimeOtp-Secret-Base32", "KPEX_PASSKEY_CREDENTIAL_ID") {
		return bw.ItemTypeLogin
	}
	return bw.ItemTypeSecureNote
}

func importLogin(entry kp.Entry, existing *bw.Login) *bw.Login {
	login := bw.Login{}
	if existing != nil {
		login = *existing
	}
	login.Username, login.Password = entry.Username, entry.Password.Value
	login.URIs = nil
	if entry.URL != "" {
		login.URIs = append(login.URIs, bw.URI{URI: entry.URL})
	}
	additional := make([]string, 0)
	for name, value := range entry.Fields {
		if strings.HasPrefix(name, "KP2A_URL_") && value.Value != "" {
			additional = append(additional, value.Value)
		}
	}
	slices.Sort(additional)
	for _, uri := range additional {
		login.URIs = append(login.URIs, bw.URI{URI: uri})
	}
	for i := range login.URIs {
		value := fieldValue(entry, fmt.Sprintf("BW.URIMatch.%d", i+1))
		if value == "" {
			continue
		}
		if match, err := strconv.Atoi(value); err == nil {
			login.URIs[i].Match = &match
		}
	}
	login.TOTP = cmp.Or(fieldValue(entry, "otp"), legacyTOTP(entry))
	if credential, ok := importPasskey(entry); ok {
		login.FIDO2Credentials = []bw.FIDO2Credential{credential}
	}
	return &login
}

func legacyTOTP(entry kp.Entry) string {
	secret := cmp.Or(fieldValue(entry, "TOTP Seed"), fieldValue(entry, "TimeOtp-Secret-Base32"))
	if secret == "" {
		return ""
	}
	return "otpauth://totp/" + entry.Title + "?secret=" + secret
}

func importPasskey(entry kp.Entry) (bw.FIDO2Credential, bool) {
	id := fieldValue(entry, "KPEX_PASSKEY_CREDENTIAL_ID")
	key := fieldValue(entry, "KPEX_PASSKEY_PRIVATE_KEY_PEM")
	if id == "" || key == "" {
		return bw.FIDO2Credential{}, false
	}
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return bw.FIDO2Credential{}, false
	}
	credential := bw.FIDO2Credential{
		CredentialID:    id,
		KeyValue:        base64.RawURLEncoding.EncodeToString(block.Bytes),
		RPID:            fieldValue(entry, "KPEX_PASSKEY_RELYING_PARTY"),
		UserName:        fieldValue(entry, "KPEX_PASSKEY_USERNAME"),
		UserHandle:      fieldValue(entry, "KPEX_PASSKEY_USER_HANDLE"),
		RPName:          fieldValue(entry, "BW.Passkey.RPName"),
		UserDisplayName: fieldValue(entry, "BW.Passkey.UserDisplayName"),
		Counter:         fieldValue(entry, "BW.Passkey.Counter"),
		Discoverable:    fieldValue(entry, "BW.Passkey.Discoverable"),
	}
	if created, err := time.Parse(time.RFC3339Nano, fieldValue(entry, "BW.Passkey.CreationDate")); err == nil {
		credential.CreationDate = created
	}
	return credential, true
}

func importCard(entry kp.Entry, existing *bw.Card) *bw.Card {
	card := bw.Card{}
	if existing != nil {
		card = *existing
	}
	card.CardholderName = cmp.Or(fieldValue(entry, "Cardholder Name"), card.CardholderName)
	card.Brand = cmp.Or(fieldValue(entry, "Card Brand"), card.Brand)
	card.Number = cmp.Or(fieldValue(entry, "Card Number"), card.Number)
	card.ExpMonth = cmp.Or(fieldValue(entry, "Expiry Month"), card.ExpMonth)
	card.ExpYear = cmp.Or(fieldValue(entry, "Expiry Year"), card.ExpYear)
	card.Code = cmp.Or(fieldValue(entry, "CVV"), card.Code)
	return &card
}

func importIdentity(entry kp.Entry, existing *bw.Identity) *bw.Identity {
	identity := bw.Identity{}
	if existing != nil {
		identity = *existing
	}
	values := map[string]*string{
		"Identity Title": &identity.Title, "First Name": &identity.FirstName, "Middle Name": &identity.MiddleName,
		"Last Name": &identity.LastName, "Address 1": &identity.Address1, "Address 2": &identity.Address2,
		"Address 3": &identity.Address3, "City": &identity.City, "State": &identity.State,
		"Postal Code": &identity.PostalCode, "Country": &identity.Country, "Company": &identity.Company,
		"Email": &identity.Email, "Phone": &identity.Phone, "SSN": &identity.SSN,
		"Identity Username": &identity.Username, "Passport Number": &identity.PassportNumber,
		"License Number": &identity.LicenseNumber,
	}
	if identity.Title == "" {
		identity.Title = fieldValue(entry, "Title")
	}
	for name, target := range values {
		if value := fieldValue(entry, name); value != "" {
			*target = value
		}
	}
	return &identity
}

func importSSHKey(entry kp.Entry, existing *bw.SSHKey) (*bw.SSHKey, bool) {
	key := bw.SSHKey{}
	if existing != nil {
		key = *existing
	}
	key.PublicKey = cmp.Or(fieldValue(entry, "SSH Public Key"), key.PublicKey)
	key.Fingerprint = cmp.Or(fieldValue(entry, "SSH Fingerprint"), key.Fingerprint)
	for _, attachment := range entry.Attachments {
		if attachment.Name == "id_ssh" {
			key.PrivateKey = string(attachment.Content)
			break
		}
	}
	return &key, key.PrivateKey != ""
}

func importStructuredData(entry kp.Entry, existing map[string]any) map[string]any {
	data := make(map[string]any, len(existing))
	for key, value := range existing {
		data[key] = value
	}
	for name, value := range entry.Fields {
		key, ok := strings.CutPrefix(name, "Bitwarden ")
		if !ok {
			continue
		}
		var decoded any
		if json.Unmarshal([]byte(value.Value), &decoded) == nil {
			data[key] = decoded
		} else {
			data[key] = value.Value
		}
	}
	return data
}

func fallbackNote(entry kp.Entry, item bw.Item) bw.Item {
	item.Type = bw.ItemTypeSecureNote
	item.Login, item.Card, item.Identity, item.SSHKey, item.Data = nil, nil, nil, nil, nil
	var text strings.Builder
	if entry.Notes != "" {
		text.WriteString(entry.Notes)
		text.WriteString("\n\n")
	}
	text.WriteString("KeePassXC entry\n")
	for _, field := range []struct{ name, value string }{
		{"Username", entry.Username}, {"Password", entry.Password.Value}, {"URL", entry.URL},
	} {
		if field.value != "" {
			fmt.Fprintf(&text, "%s: %s\n", field.name, field.value)
		}
	}
	names := slices.Sorted(maps.Keys(entry.Fields))
	for _, name := range names {
		fmt.Fprintf(&text, "%s: %s\n", name, entry.Fields[name].Value)
	}
	item.Notes = strings.TrimSpace(text.String())
	return item
}

func importFields(entry kp.Entry, consumed map[string]struct{}) []bw.Field {
	names := slices.Sorted(maps.Keys(entry.Fields))
	for _, name := range names {
		if _, skipped := consumed[name]; skipped || strings.HasPrefix(name, "BW.") || strings.HasPrefix(name, "KP2A_URL_") {
			continue
		}
		for _, prefix := range []string{"BW.FieldType.", "BW.LinkedField."} {
			metadata := prefix + name
			if encoded := fieldValue(entry, metadata); encoded != "" {
				if _, err := strconv.Atoi(encoded); err == nil {
					consumed[metadata] = struct{}{}
				}
			}
		}
	}
	fields := make([]bw.Field, 0, len(names))
	for _, name := range names {
		_, explicitlyConsumed := consumed[name]
		if explicitlyConsumed || strings.HasPrefix(name, "KP2A_URL_") || name == "KP.SourceJSON" {
			continue
		}
		value := entry.Fields[name]
		fieldType := 0
		if value.Protected {
			fieldType = 1
		}
		if encoded := fieldValue(entry, "BW.FieldType."+name); encoded != "" {
			if parsed, err := strconv.Atoi(encoded); err == nil {
				fieldType = parsed
			}
		}
		field := bw.Field{Name: name, Value: value.Value, Type: fieldType}
		if encoded := fieldValue(entry, "BW.LinkedField."+name); encoded != "" {
			if parsed, err := strconv.Atoi(encoded); err == nil {
				field.Linked = &parsed
			}
		}
		fields = append(fields, field)
	}
	return fields
}

func addPreservationFields(item *bw.Item, entry kp.Entry) {
	if len(entry.History) > 0 {
		if raw, err := json.Marshal(entry.History); err == nil {
			item.Fields = append(item.Fields, bw.Field{Name: "KP.HistoryJSON", Value: string(raw), Type: 1})
		}
	}
	if entry.Expires != nil {
		item.Fields = append(item.Fields, bw.Field{Name: "KP.Expires", Value: entry.Expires.Format("2006-01-02T15:04:05.999999999Z07:00"), Type: 1})
	}
	remainingTags := make([]string, 0, len(entry.Tags))
	for _, tag := range entry.Tags {
		if tag != "Favorite" && tag != "Archived" && tag != "Passkey" {
			remainingTags = append(remainingTags, tag)
		}
	}
	if len(remainingTags) > 0 {
		item.Fields = append(item.Fields, bw.Field{Name: "KP.Tags", Value: strings.Join(remainingTags, ", ")})
	}
	for _, value := range []struct {
		name  string
		value string
	}{
		{"KP.UUID", entry.UUID},
		{"KP.Created", formatOptionalTime(entry.Created)},
		{"KP.Modified", formatOptionalTime(entry.Modified)},
		{"KP.Accessed", formatOptionalTime(entry.Accessed)},
		{"KP.Icon", strconv.Itoa(entry.Icon)},
		{"KP.IconUUID", entry.IconUUID},
		{"KP.Foreground", entry.Foreground},
		{"KP.Background", entry.Background},
		{"KP.OverrideURL", entry.OverrideURL},
	} {
		if value.value != "" && (value.name != "KP.Icon" || entry.Icon != 0) {
			item.Fields = append(item.Fields, bw.Field{Name: value.name, Value: value.value, Type: 1})
		}
	}
	if entry.AutoType.Enabled || entry.AutoType.Obfuscation != 0 || entry.AutoType.Sequence != "" || len(entry.AutoType.Windows) > 0 {
		if raw, err := json.Marshal(entry.AutoType); err == nil {
			item.Fields = append(item.Fields, bw.Field{Name: "KP.AutoTypeJSON", Value: string(raw), Type: 1})
		}
	}
	if len(entry.CustomData) > 0 {
		if raw, err := json.Marshal(entry.CustomData); err == nil {
			item.Fields = append(item.Fields, bw.Field{Name: "KP.CustomDataJSON", Value: string(raw), Type: 1})
		}
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}

func sourceJSON(entry kp.Entry, folder string) ([]byte, error) {
	type descriptor struct {
		Name   string `json:"name"`
		Size   int    `json:"size"`
		SHA256 string `json:"sha256"`
	}
	attachments := make([]descriptor, 0, len(entry.Attachments))
	for _, attachment := range entry.Attachments {
		sum := sha256.Sum256(attachment.Content)
		attachments = append(attachments, descriptor{attachment.Name, len(attachment.Content), hex.EncodeToString(sum[:])})
	}
	clone := entry
	clone.Attachments = nil
	return json.Marshal(struct {
		Entry       kp.Entry     `json:"entry"`
		GroupPath   string       `json:"groupPath"`
		Attachments []descriptor `json:"attachments,omitzero"`
	}{clone, folder, attachments})
}

func importAttachments(values []kp.Attachment) []bw.Attachment {
	result := make([]bw.Attachment, 0, len(values))
	for _, value := range values {
		result = append(result, bw.Attachment{FileName: value.Name, Size: int64(len(value.Content)), Content: slices.Clone(value.Content)})
	}
	return result
}

func removeSSHControlAttachments(values []bw.Attachment) []bw.Attachment {
	return slices.DeleteFunc(values, func(value bw.Attachment) bool {
		return value.FileName == "id_ssh" || value.FileName == "KeeAgent.settings"
	})
}

func consumedFields(itemType bw.ItemType) map[string]struct{} {
	names := []string{"BW.SourceJSON", "BW.ItemID", "BW.ItemType"}
	switch itemType {
	case bw.ItemTypeLogin:
		names = append(names, "otp", "TOTP Seed", "TimeOtp-Secret-Base32")
	case bw.ItemTypeCard:
		names = append(names, "Cardholder Name", "Card Brand", "Card Number", "Expiry Month", "Expiry Year", "CVV")
	case bw.ItemTypeIdentity:
		names = append(names, "Identity Title", "Title", "First Name", "Middle Name", "Last Name", "Address 1", "Address 2", "Address 3", "City", "State", "Postal Code", "Country", "Company", "Email", "Phone", "SSN", "Identity Username", "Passport Number", "License Number")
	case bw.ItemTypeSSHKey:
		names = append(names, "SSH Public Key", "SSH Fingerprint")
	case bw.ItemTypeSecureNote, bw.ItemTypeBankAccount, bw.ItemTypeDriversLicense, bw.ItemTypePassport:
	}
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		result[name] = struct{}{}
	}
	return result
}

func passkeyFieldNames() []string {
	return []string{"KPEX_PASSKEY_CREDENTIAL_ID", "KPEX_PASSKEY_PRIVATE_KEY_PEM", "KPEX_PASSKEY_RELYING_PARTY", "KPEX_PASSKEY_USERNAME", "KPEX_PASSKEY_USER_HANDLE", "BW.Passkey.RPName", "BW.Passkey.UserDisplayName", "BW.Passkey.Counter", "BW.Passkey.Discoverable"}
}

func fieldValue(entry kp.Entry, name string) string { return entry.Fields[name].Value }

func hasAnyField(entry kp.Entry, names ...string) bool {
	return slices.ContainsFunc(names, func(name string) bool { return fieldValue(entry, name) != "" })
}
