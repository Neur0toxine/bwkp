//go:build !native

package bwapi

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Neur0toxine/bwkp/internal/native"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
)

func TestDecodeLoginChallenge(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		check   func(error) bool
	}{
		{
			name:    "two factor",
			payload: `{"type":"two-factor","providers":["authenticator","email"]}`,
			check: func(err error) bool {
				challenge, ok := errors.AsType[*TwoFactorRequiredError](err)
				return ok && slices.Equal(challenge.Providers, []string{"authenticator", "email"})
			},
		},
		{
			name:    "device verification",
			payload: `{"type":"device-verification","message":"new device verification required"}`,
			check: func(err error) bool {
				challenge, ok := errors.AsType[*DeviceVerificationRequiredError](err)
				return ok && challenge.Message == "new device verification required"
			},
		},
		{name: "unknown", payload: `{"type":"captcha"}`, check: func(err error) bool { return err != nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := decodeLoginChallenge([]byte(test.payload)); !test.check(err) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestNativeAdapterReportsUnavailableWithoutNativeBuild(t *testing.T) {
	_, err := NewNativeClient().Login(t.Context(), LoginRequest{Email: "a@example.test"})
	if !errors.Is(err, native.ErrUnavailable) {
		t.Fatalf("error = %v", err)
	}
	session := &nativeSession{handle: 1}
	if _, err := session.Sync(t.Context()); !errors.Is(err, native.ErrUnavailable) {
		t.Fatalf("sync error = %v", err)
	}
	reader, err := session.DownloadAttachment(t.Context(), bw.Item{ID: "item"}, bw.Attachment{ID: "attachment"})
	if reader != nil || !errors.Is(err, native.ErrUnavailable) {
		t.Fatalf("reader=%v error=%v", reader, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.currentHandle(); err == nil {
		t.Fatal("closed session returned a handle")
	}
}

func TestNativeAdapterHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewNativeClient().Login(ctx, LoginRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
