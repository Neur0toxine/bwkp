// Package bwapi defines the public boundary to Bitwarden and Vaultwarden.
package bwapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
)

type Region string

const (
	RegionUS Region = "us"
	RegionEU Region = "eu"
)

type Endpoints struct {
	VaultURL    string `json:"vaultUrl"`
	APIURL      string `json:"apiUrl"`
	IdentityURL string `json:"identityUrl"`
	CACertPEM   []byte `json:"caCertPem,omitzero"`
}

type LoginRequest struct {
	Endpoints      Endpoints `json:"endpoints"`
	Email          string    `json:"email"`
	MasterPassword []byte    `json:"masterPassword"`
	TOTP           string    `json:"totp,omitzero"`
}

type TwoFactorRequiredError struct {
	Providers []string
}

func (e *TwoFactorRequiredError) Error() string {
	return "two-factor authentication is required (available: " + strings.Join(e.Providers, ", ") + ")"
}

type Client interface {
	Login(context.Context, LoginRequest) (Session, error)
}

type Session interface {
	Sync(context.Context) (bw.Vault, error)
	DownloadAttachment(context.Context, bw.Item, bw.Attachment) (io.ReadCloser, error)
	Close() error
}

type ImportSession interface {
	Session
	CreateFolder(context.Context, string) (bw.Folder, error)
	CreateItem(context.Context, bw.Item, string) (string, error)
	UpdateItem(context.Context, string, bw.Item, string) error
	TrashItem(context.Context, string) error
	RestoreItem(context.Context, string) error
	ArchiveItem(context.Context, string) error
	UnarchiveItem(context.Context, string) error
	DeleteAttachment(context.Context, string, string) error
	UploadAttachment(context.Context, string, bw.Attachment) error
}

func ResolveEndpoints(region Region, server, apiURL, identityURL string, caCert []byte) (Endpoints, error) {
	if server != "" && region != "" {
		return Endpoints{}, errors.New("region and server are mutually exclusive")
	}
	var endpoints Endpoints
	switch region {
	case RegionUS:
		endpoints = Endpoints{VaultURL: "https://vault.bitwarden.com", APIURL: "https://api.bitwarden.com", IdentityURL: "https://identity.bitwarden.com"}
	case RegionEU:
		endpoints = Endpoints{VaultURL: "https://vault.bitwarden.eu", APIURL: "https://api.bitwarden.eu", IdentityURL: "https://identity.bitwarden.eu"}
	case "":
		if server == "" {
			return Endpoints{}, errors.New("one of region or server is required")
		}
		base, err := normalizeURL(server)
		if err != nil {
			return Endpoints{}, fmt.Errorf("server: %w", err)
		}
		endpoints = Endpoints{VaultURL: base, APIURL: base + "/api", IdentityURL: base + "/identity"}
	default:
		return Endpoints{}, fmt.Errorf("unsupported region %q", region)
	}
	if apiURL != "" {
		var err error
		endpoints.APIURL, err = normalizeURL(apiURL)
		if err != nil {
			return Endpoints{}, fmt.Errorf("API URL: %w", err)
		}
	}
	if identityURL != "" {
		var err error
		endpoints.IdentityURL, err = normalizeURL(identityURL)
		if err != nil {
			return Endpoints{}, fmt.Errorf("identity URL: %w", err)
		}
	}
	endpoints.CACertPEM = caCert
	return endpoints, nil
}

func normalizeURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("URL must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("URL must contain only scheme, host, port, and optional path")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
