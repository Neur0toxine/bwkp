//go:build !native

package bwapi

import (
	"context"
	"errors"
	"testing"

	"github.com/Neur0toxine/bitwarden-keepass-exporter/internal/native"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/dto/bw"
)

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
