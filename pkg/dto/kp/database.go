// Package kp contains a writer-neutral KeePass database model.
package kp

import "time"

type Database struct {
	Name string `json:"name"`
	Root Group  `json:"root"`
}

type Group struct {
	Name    string  `json:"name"`
	Groups  []Group `json:"groups,omitzero"`
	Entries []Entry `json:"entries,omitzero"`
}

type Entry struct {
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
