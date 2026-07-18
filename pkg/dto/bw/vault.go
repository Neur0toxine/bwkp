// Package bw contains decrypted, provider-neutral Bitwarden vault data.
package bw

import (
	"encoding/json"
	"time"
)

// ItemType identifies a Bitwarden cipher kind.
type ItemType string

const (
	ItemTypeLogin          ItemType = "login"
	ItemTypeSecureNote     ItemType = "secureNote"
	ItemTypeCard           ItemType = "card"
	ItemTypeIdentity       ItemType = "identity"
	ItemTypeSSHKey         ItemType = "sshKey"
	ItemTypeBankAccount    ItemType = "bankAccount"
	ItemTypeDriversLicense ItemType = "driversLicense"
	ItemTypePassport       ItemType = "passport"
)

// Vault is one complete decrypted sync snapshot.
type Vault struct {
	Source        Source         `json:"source"`
	Folders       []Folder       `json:"folders,omitzero"`
	Collections   []Collection   `json:"collections,omitzero"`
	Organizations []Organization `json:"organizations,omitzero"`
	Items         []Item         `json:"items,omitzero"`
}

// Source describes the endpoint and account that produced a snapshot.
type Source struct {
	Server   string    `json:"server"`
	Email    string    `json:"email"`
	UserID   string    `json:"userId,omitzero"`
	SyncedAt time.Time `json:"syncedAt"`
}

type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Collection struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
}

// Item contains common cipher properties and exactly one type-specific payload.
type Item struct {
	ID              string            `json:"id"`
	OrganizationID  string            `json:"organizationId,omitzero"`
	FolderID        string            `json:"folderId,omitzero"`
	CollectionIDs   []string          `json:"collectionIds,omitzero"`
	Type            ItemType          `json:"type"`
	Name            string            `json:"name"`
	Notes           string            `json:"notes,omitzero"`
	Favorite        bool              `json:"favorite,omitzero"`
	Reprompt        bool              `json:"reprompt,omitzero"`
	CreationDate    time.Time         `json:"creationDate"`
	RevisionDate    time.Time         `json:"revisionDate"`
	DeletedDate     *time.Time        `json:"deletedDate,omitzero"`
	ArchivedDate    *time.Time        `json:"archivedDate,omitzero"`
	Login           *Login            `json:"login,omitzero"`
	Card            *Card             `json:"card,omitzero"`
	Identity        *Identity         `json:"identity,omitzero"`
	SSHKey          *SSHKey           `json:"sshKey,omitzero"`
	Fields          []Field           `json:"fields,omitzero"`
	Attachments     []Attachment      `json:"attachments,omitzero"`
	PasswordHistory []PasswordHistory `json:"passwordHistory,omitzero"`
	Data            map[string]any    `json:"data,omitzero"`
	SourceJSON      json.RawMessage   `json:"-"`
}

type Login struct {
	Username         string            `json:"username,omitzero"`
	Password         string            `json:"password,omitzero"`
	TOTP             string            `json:"totp,omitzero"`
	URIs             []URI             `json:"uris,omitzero"`
	FIDO2Credentials []FIDO2Credential `json:"fido2Credentials,omitzero"`
}

type URI struct {
	URI   string `json:"uri"`
	Match *int   `json:"match,omitzero"`
}

type FIDO2Credential struct {
	CredentialID    string    `json:"credentialId"`
	KeyValue        string    `json:"keyValue"`
	RPID            string    `json:"rpId"`
	RPName          string    `json:"rpName,omitzero"`
	UserHandle      string    `json:"userHandle"`
	UserName        string    `json:"userName"`
	UserDisplayName string    `json:"userDisplayName,omitzero"`
	Counter         string    `json:"counter,omitzero"`
	Discoverable    string    `json:"discoverable,omitzero"`
	CreationDate    time.Time `json:"creationDate,omitzero"`
}

type Field struct {
	Name   string `json:"name,omitzero"`
	Value  string `json:"value,omitzero"`
	Type   int    `json:"type"`
	Linked *int   `json:"linkedId,omitzero"`
}

type Card struct {
	CardholderName string `json:"cardholderName,omitzero"`
	Brand          string `json:"brand,omitzero"`
	Number         string `json:"number,omitzero"`
	ExpMonth       string `json:"expMonth,omitzero"`
	ExpYear        string `json:"expYear,omitzero"`
	Code           string `json:"code,omitzero"`
}

type Identity struct {
	Title          string `json:"title,omitzero"`
	FirstName      string `json:"firstName,omitzero"`
	MiddleName     string `json:"middleName,omitzero"`
	LastName       string `json:"lastName,omitzero"`
	Address1       string `json:"address1,omitzero"`
	Address2       string `json:"address2,omitzero"`
	Address3       string `json:"address3,omitzero"`
	City           string `json:"city,omitzero"`
	State          string `json:"state,omitzero"`
	PostalCode     string `json:"postalCode,omitzero"`
	Country        string `json:"country,omitzero"`
	Company        string `json:"company,omitzero"`
	Email          string `json:"email,omitzero"`
	Phone          string `json:"phone,omitzero"`
	SSN            string `json:"ssn,omitzero"`
	Username       string `json:"username,omitzero"`
	PassportNumber string `json:"passportNumber,omitzero"`
	LicenseNumber  string `json:"licenseNumber,omitzero"`
}

type SSHKey struct {
	PrivateKey  string `json:"privateKey"`
	PublicKey   string `json:"publicKey,omitzero"`
	Fingerprint string `json:"fingerprint,omitzero"`
}

type Attachment struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size,omitzero"`
	Key      string `json:"key,omitzero"`
	URL      string `json:"url,omitzero"`
	Content  []byte `json:"-"`
}

type PasswordHistory struct {
	Password     string    `json:"password"`
	LastUsedDate time.Time `json:"lastUsedDate"`
}
