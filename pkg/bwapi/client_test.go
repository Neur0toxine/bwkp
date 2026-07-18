package bwapi_test

import (
	"testing"

	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/bwapi"
)

func TestResolveEndpoints(t *testing.T) {
	tests := []struct {
		name, server string
		region       bwapi.Region
		wantAPI      string
		wantError    bool
	}{
		{name: "US", region: bwapi.RegionUS, wantAPI: "https://api.bitwarden.com"},
		{name: "EU", region: bwapi.RegionEU, wantAPI: "https://api.bitwarden.eu"},
		{name: "self hosted", server: "https://vault.example.test/", wantAPI: "https://vault.example.test/api"},
		{name: "mutually exclusive", region: bwapi.RegionUS, server: "https://example.test", wantError: true},
		{name: "credentials forbidden", server: "https://user@example.test", wantError: true},
		{name: "invalid scheme", server: "ftp://example.test", wantError: true},
		{name: "missing selection", wantError: true},
		{name: "unknown region", region: "apac", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := bwapi.ResolveEndpoints(test.region, test.server, "", "", nil)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v", err)
			}
			if err == nil && got.APIURL != test.wantAPI {
				t.Fatalf("API URL = %q", got.APIURL)
			}
		})
	}
}

func TestEndpointOverrides(t *testing.T) {
	got, err := bwapi.ResolveEndpoints(bwapi.RegionUS, "", "https://api.example.test/", "https://identity.example.test", []byte("CA"))
	if err != nil {
		t.Fatal(err)
	}
	if got.APIURL != "https://api.example.test" || string(got.CACertPEM) != "CA" {
		t.Fatalf("endpoints = %+v", got)
	}
	if _, err := bwapi.ResolveEndpoints(bwapi.RegionUS, "", "://bad", "", nil); err == nil {
		t.Fatal("invalid override accepted")
	}
}
