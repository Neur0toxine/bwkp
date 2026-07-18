// Package kp contains a writer-neutral KeePass database model.
package kp

import "time"

type Database struct {
	Name string `json:"name"`
	Root Group  `json:"root"`
}

type Group struct {
	UUID       string  `json:"uuid,omitzero"`
	Name       string  `json:"name"`
	RecycleBin bool    `json:"recycleBin,omitzero"`
	Templates  bool    `json:"templates,omitzero"`
	Groups     []Group `json:"groups,omitzero"`
	Entries    []Entry `json:"entries,omitzero"`
}

type Entry struct {
	UUID        string           `json:"uuid,omitzero"`
	Title       string           `json:"title"`
	Username    string           `json:"username,omitzero"`
	Password    ProtectedString  `json:"password,omitzero"`
	URL         string           `json:"url,omitzero"`
	Notes       string           `json:"notes,omitzero"`
	Tags        []string         `json:"tags,omitzero"`
	Fields      map[string]Value `json:"fields,omitzero"`
	Attachments []Attachment     `json:"attachments,omitzero"`
	History     []Entry          `json:"history,omitzero"`
	Created     time.Time        `json:"created,omitzero"`
	Modified    time.Time        `json:"modified,omitzero"`
	Accessed    time.Time        `json:"accessed,omitzero"`
	Expires     *time.Time       `json:"expires,omitzero"`
	Recycled    bool             `json:"recycled,omitzero"`
	Icon        int              `json:"icon,omitzero"`
	IconUUID    string           `json:"iconUuid,omitzero"`
	Foreground  string           `json:"foreground,omitzero"`
	Background  string           `json:"background,omitzero"`
	OverrideURL string           `json:"overrideUrl,omitzero"`
	AutoType    AutoType         `json:"autoType,omitzero"`
	CustomData  map[string]Value `json:"customData,omitzero"`
}

type AutoType struct {
	Enabled     bool              `json:"enabled,omitzero"`
	Obfuscation int               `json:"obfuscation,omitzero"`
	Sequence    string            `json:"sequence,omitzero"`
	Windows     map[string]string `json:"windows,omitzero"`
}

type ProtectedString struct {
	Value string `json:"value,omitzero"`
}

type Value struct {
	Value     string `json:"value"`
	Protected bool   `json:"protected,omitzero"`
}

type Attachment struct {
	Name    string `json:"name"`
	Content []byte `json:"content"`
}
