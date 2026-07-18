// Package convert implements the pure Bitwarden-to-KeePass conversion.
package convert

import (
	"cmp"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/dto/bw"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/dto/kp"
)

var ErrUnsupportedItem = errors.New("unsupported Bitwarden item")

type Report struct {
	Items       int
	Entries     int
	Attachments int
	Passkeys    int
}

type Converter struct{}

func New() *Converter { return &Converter{} }

func (c *Converter) Convert(vault bw.Vault) (kp.Database, Report, error) {
	return c.ConvertWithProgress(vault, nil)
}

func (c *Converter) ConvertWithProgress(vault bw.Vault, progress func(completed, total int)) (kp.Database, Report, error) {
	db := kp.Database{
		Name: "Bitwarden Export",
		Root: kp.Group{Name: "Bitwarden Export"},
	}

	folders := indexFolders(vault.Folders)
	organizations := indexOrganizations(vault.Organizations)
	collections := indexCollections(vault.Collections)
	items := slices.Clone(vault.Items)
	slices.SortFunc(items, func(a, b bw.Item) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})

	report := Report{Items: len(items)}
	for index, item := range items {
		entries, err := c.convertItem(item)
		if err != nil {
			return kp.Database{}, Report{}, fmt.Errorf("convert item %q (%s): %w", item.Name, item.ID, err)
		}
		path := itemPath(item, folders, organizations, collections)
		group := ensureGroup(&db.Root, path)
		group.Entries = append(group.Entries, entries...)
		report.Entries += len(entries)
		for _, entry := range entries {
			report.Attachments += len(entry.Attachments)
		}
		if item.Login != nil {
			report.Passkeys += len(item.Login.FIDO2Credentials)
		}
		if progress != nil {
			progress(index+1, len(items))
		}
	}

	sortGroup(&db.Root)
	return db, report, nil
}

func (c *Converter) convertItem(item bw.Item) ([]kp.Entry, error) {
	entry := kp.Entry{
		Title:    cmp.Or(item.Name, "Untitled"),
		Notes:    item.Notes,
		Fields:   make(map[string]kp.Value),
		Created:  item.CreationDate,
		Modified: item.RevisionDate,
		Accessed: item.RevisionDate,
	}
	if item.Favorite {
		entry.Tags = append(entry.Tags, "Favorite")
	}
	if item.ArchivedDate != nil {
		entry.Tags = append(entry.Tags, "Archived")
	}

	switch item.Type {
	case bw.ItemTypeLogin:
		if item.Login == nil {
			return nil, fmt.Errorf("%w: login payload is missing", ErrUnsupportedItem)
		}
		mapLogin(&entry, *item.Login)
	case bw.ItemTypeSecureNote:
	case bw.ItemTypeCard:
		if item.Card == nil {
			return nil, fmt.Errorf("%w: card payload is missing", ErrUnsupportedItem)
		}
		mapCard(&entry, *item.Card)
	case bw.ItemTypeIdentity:
		if item.Identity == nil {
			return nil, fmt.Errorf("%w: identity payload is missing", ErrUnsupportedItem)
		}
		mapIdentity(&entry, *item.Identity)
	case bw.ItemTypeSSHKey:
		if item.SSHKey == nil {
			return nil, fmt.Errorf("%w: SSH payload is missing", ErrUnsupportedItem)
		}
		mapSSHKey(&entry, *item.SSHKey)
	case bw.ItemTypeBankAccount, bw.ItemTypeDriversLicense, bw.ItemTypePassport:
		mapStructuredData(&entry, item.Data)
	default:
		return nil, fmt.Errorf("%w: type %q", ErrUnsupportedItem, item.Type)
	}

	mapCustomFields(&entry, item.Fields)
	mapAttachments(&entry, item.Attachments)
	mapHistory(&entry, item.PasswordHistory)
	if err := addSourceMetadata(&entry, item); err != nil {
		return nil, err
	}

	if item.Login == nil || len(item.Login.FIDO2Credentials) == 0 {
		return []kp.Entry{entry}, nil
	}

	entries := make([]kp.Entry, 0, len(item.Login.FIDO2Credentials))
	for i, credential := range item.Login.FIDO2Credentials {
		clone := cloneEntry(entry)
		if len(item.Login.FIDO2Credentials) > 1 {
			clone.Title = fmt.Sprintf("%s [Passkey %d]", clone.Title, i+1)
		}
		if err := mapPasskey(&clone, credential); err != nil {
			return nil, fmt.Errorf("passkey %d: %w", i+1, err)
		}
		clone.Tags = append(clone.Tags, "Passkey")
		entries = append(entries, clone)
	}
	return entries, nil
}

func mapLogin(entry *kp.Entry, login bw.Login) {
	entry.Username = login.Username
	entry.Password = kp.ProtectedString{Value: login.Password}
	if len(login.URIs) > 0 {
		entry.URL = login.URIs[0].URI
		for i, uri := range login.URIs[1:] {
			entry.Fields[fmt.Sprintf("KP2A_URL_%d", i+1)] = kp.Value{Value: uri.URI}
		}
	}
	if login.TOTP != "" {
		entry.Fields["otp"] = kp.Value{Value: normalizeTOTP(entry.Title, login.TOTP), Protected: true}
	}
}

func normalizeTOTP(title, value string) string {
	if strings.HasPrefix(strings.ToLower(value), "otpauth://") {
		return value
	}
	params := url.Values{"secret": []string{value}, "issuer": []string{"Bitwarden"}}
	return "otpauth://totp/" + url.PathEscape(title) + "?" + params.Encode()
}

func mapCard(entry *kp.Entry, card bw.Card) {
	addProtected(entry, "Cardholder Name", card.CardholderName)
	addProtected(entry, "Card Brand", card.Brand)
	addProtected(entry, "Card Number", card.Number)
	addProtected(entry, "Expiry Month", card.ExpMonth)
	addProtected(entry, "Expiry Year", card.ExpYear)
	addProtected(entry, "CVV", card.Code)
}

func mapIdentity(entry *kp.Entry, identity bw.Identity) {
	values := []struct{ name, value string }{
		{"Title", identity.Title}, {"First Name", identity.FirstName},
		{"Middle Name", identity.MiddleName}, {"Last Name", identity.LastName},
		{"Address 1", identity.Address1}, {"Address 2", identity.Address2},
		{"Address 3", identity.Address3}, {"City", identity.City},
		{"State", identity.State}, {"Postal Code", identity.PostalCode},
		{"Country", identity.Country}, {"Company", identity.Company},
		{"Email", identity.Email}, {"Phone", identity.Phone}, {"SSN", identity.SSN},
		{"Identity Username", identity.Username}, {"Passport Number", identity.PassportNumber},
		{"License Number", identity.LicenseNumber},
	}
	for _, value := range values {
		addProtected(entry, value.name, value.value)
	}
}

func mapSSHKey(entry *kp.Entry, key bw.SSHKey) {
	addProtected(entry, "SSH Public Key", key.PublicKey)
	addProtected(entry, "SSH Fingerprint", key.Fingerprint)
	entry.Attachments = append(entry.Attachments,
		kp.Attachment{Name: "id_ssh", Content: []byte(key.PrivateKey)},
		kp.Attachment{Name: "KeeAgent.settings", Content: []byte(keeAgentSettings())},
	)
}

func keeAgentSettings() string {
	return `<?xml version="1.0" encoding="UTF-8"?><EntrySettings><AllowUseOfSshKey>true</AllowUseOfSshKey><AddAtDatabaseOpen>true</AddAtDatabaseOpen><RemoveAtDatabaseClose>true</RemoveAtDatabaseClose><UseConfirmConstraint>false</UseConfirmConstraint><UseLifetimeConstraint>false</UseLifetimeConstraint><LifetimeConstraintDuration>600</LifetimeConstraintDuration><Location>Attachment</Location><AttachmentName>id_ssh</AttachmentName></EntrySettings>`
}

func mapStructuredData(entry *kp.Entry, data map[string]any) {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		encoded, _ := json.Marshal(data[key])
		addProtected(entry, "Bitwarden "+key, string(encoded))
	}
}

func mapCustomFields(entry *kp.Entry, fields []bw.Field) {
	for i, field := range fields {
		name := cmp.Or(strings.TrimSpace(field.Name), fmt.Sprintf("Bitwarden Field %d", i+1))
		name = uniqueFieldName(entry.Fields, name)
		entry.Fields[name] = kp.Value{Value: field.Value, Protected: field.Type == 1}
		if field.Linked != nil {
			entry.Fields[uniqueFieldName(entry.Fields, "BW.LinkedField."+name)] = kp.Value{
				Value: strconv.Itoa(*field.Linked), Protected: true,
			}
		}
	}
}

func mapAttachments(entry *kp.Entry, attachments []bw.Attachment) {
	used := make(map[string]struct{}, len(entry.Attachments)+len(attachments))
	for _, attachment := range entry.Attachments {
		used[attachment.Name] = struct{}{}
	}
	for i, attachment := range attachments {
		name := cmp.Or(attachment.FileName, fmt.Sprintf("attachment-%d", i+1))
		name = uniqueName(used, name)
		entry.Attachments = append(entry.Attachments, kp.Attachment{Name: name, Content: slices.Clone(attachment.Content)})
	}
}

func mapHistory(entry *kp.Entry, history []bw.PasswordHistory) {
	for _, password := range history {
		previous := cloneEntry(*entry)
		previous.Password = kp.ProtectedString{Value: password.Password}
		previous.Modified = password.LastUsedDate
		previous.History = nil
		entry.History = append(entry.History, previous)
	}
}

func mapPasskey(entry *kp.Entry, credential bw.FIDO2Credential) error {
	credentialID, err := credentialID(credential.CredentialID)
	if err != nil {
		return fmt.Errorf("credential ID: %w", err)
	}
	privateKey, err := decodeBase64URL(credential.KeyValue)
	if err != nil {
		return fmt.Errorf("private key: %w", err)
	}
	entry.Fields["KPEX_PASSKEY_CREDENTIAL_ID"] = kp.Value{Value: credentialID, Protected: true}
	entry.Fields["KPEX_PASSKEY_PRIVATE_KEY_PEM"] = kp.Value{
		Value: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})), Protected: true,
	}
	entry.Fields["KPEX_PASSKEY_USERNAME"] = kp.Value{Value: credential.UserName}
	entry.Fields["KPEX_PASSKEY_RELYING_PARTY"] = kp.Value{Value: credential.RPID}
	entry.Fields["KPEX_PASSKEY_USER_HANDLE"] = kp.Value{Value: credential.UserHandle, Protected: true}
	return nil
}

func credentialID(value string) (string, error) {
	if decoded, err := hex.DecodeString(value); err == nil {
		return base64.RawURLEncoding.EncodeToString(decoded), nil
	}
	decoded, err := decodeBase64URL(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(decoded), nil
}

func decodeBase64URL(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding,
	} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid base64 value")
}

func addSourceMetadata(entry *kp.Entry, item bw.Item) error {
	raw := item.SourceJSON
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(item)
		if err != nil {
			return fmt.Errorf("serialize source metadata: %w", err)
		}
	}
	entry.Fields["BW.SourceJSON"] = kp.Value{Value: string(raw), Protected: true}
	metadata := map[string]string{
		"BW.ItemID": item.ID, "BW.ItemType": string(item.Type),
		"BW.OrganizationID": item.OrganizationID, "BW.FolderID": item.FolderID,
		"BW.Reprompt": strconv.FormatBool(item.Reprompt),
	}
	if encoded, err := json.Marshal(item.CollectionIDs); err == nil {
		metadata["BW.CollectionIDs"] = string(encoded)
	}
	for key, value := range metadata {
		if value != "" {
			entry.Fields[key] = kp.Value{Value: value, Protected: true}
		}
	}
	return nil
}

func addProtected(entry *kp.Entry, name, value string) {
	if value != "" {
		entry.Fields[uniqueFieldName(entry.Fields, name)] = kp.Value{Value: value, Protected: true}
	}
}

func uniqueFieldName(fields map[string]kp.Value, base string) string {
	if _, exists := fields[base]; !exists {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s (%d)", base, suffix)
		if _, exists := fields[candidate]; !exists {
			return candidate
		}
	}
}

func uniqueName(used map[string]struct{}, base string) string {
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s (%d)", base, suffix)
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func itemPath(item bw.Item, folders map[string]string, organizations map[string]string, collections map[string]bw.Collection) []string {
	path := make([]string, 0, 6)
	if item.DeletedDate != nil {
		path = append(path, "Trash")
	}
	if item.OrganizationID == "" {
		path = append(path, "Personal")
		if folder := folders[item.FolderID]; folder != "" {
			path = append(path, pathParts(folder)...)
		} else {
			path = append(path, "Unfiled")
		}
		return path
	}
	path = append(path, "Organizations", cmp.Or(organizations[item.OrganizationID], item.OrganizationID, "Unknown Organization"))
	choices := make([]string, 0, len(item.CollectionIDs))
	for _, id := range item.CollectionIDs {
		if collection, ok := collections[id]; ok && collection.OrganizationID == item.OrganizationID {
			choices = append(choices, collection.Name)
		}
	}
	slices.Sort(choices)
	if len(choices) == 0 {
		return append(path, "Unfiled")
	}
	return append(path, pathParts(choices[0])...)
}

func pathParts(value string) []string {
	parts := make([]string, 0, strings.Count(value, "/")+1)
	for part := range strings.SplitSeq(value, "/") {
		if part = strings.TrimSpace(part); part != "" && part != "." && part != ".." {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return []string{"Unfiled"}
	}
	return parts
}

func ensureGroup(group *kp.Group, path []string) *kp.Group {
	current := group
	for _, name := range path {
		index := slices.IndexFunc(current.Groups, func(candidate kp.Group) bool { return candidate.Name == name })
		if index < 0 {
			current.Groups = append(current.Groups, kp.Group{Name: name})
			index = len(current.Groups) - 1
		}
		current = &current.Groups[index]
	}
	return current
}

func sortGroup(group *kp.Group) {
	slices.SortFunc(group.Groups, func(a, b kp.Group) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(group.Entries, func(a, b kp.Entry) int { return cmp.Compare(a.Title, b.Title) })
	for i := range group.Groups {
		sortGroup(&group.Groups[i])
	}
}

func cloneEntry(entry kp.Entry) kp.Entry {
	clone := entry
	clone.Tags = slices.Clone(entry.Tags)
	clone.Attachments = slices.Clone(entry.Attachments)
	clone.History = slices.Clone(entry.History)
	clone.Fields = make(map[string]kp.Value, len(entry.Fields))
	for key, value := range entry.Fields {
		clone.Fields[key] = value
	}
	return clone
}

func indexFolders(folders []bw.Folder) map[string]string {
	result := make(map[string]string, len(folders))
	for _, folder := range folders {
		result[folder.ID] = folder.Name
	}
	return result
}

func indexOrganizations(organizations []bw.Organization) map[string]string {
	result := make(map[string]string, len(organizations))
	for _, organization := range organizations {
		result[organization.ID] = organization.Name
	}
	return result
}

func indexCollections(collections []bw.Collection) map[string]bw.Collection {
	result := make(map[string]bw.Collection, len(collections))
	for _, collection := range collections {
		result[collection.ID] = collection
	}
	return result
}
