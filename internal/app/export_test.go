package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/Neur0toxine/bwkp/pkg/bwapi"
	"github.com/Neur0toxine/bwkp/pkg/convert"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
	"github.com/Neur0toxine/bwkp/pkg/kpdb"
)

func TestExportRetriesTOTPDownloadsAttachmentsAndWrites(t *testing.T) {
	client := &fakeClient{session: &fakeSession{vault: bw.Vault{Items: []bw.Item{{
		ID: "item", Name: "Note", Type: bw.ItemTypeSecureNote,
		Attachments: []bw.Attachment{{ID: "attachment", FileName: "file.bin"}},
	}}}}}
	writer := &fakeWriter{}
	progress := &fakeProgress{}
	exporter := New(client, convert.New(), writer)
	password := []byte("master")
	dbPassword := []byte("database")
	report, err := exporter.Export(t.Context(), Request{
		Login:  bwapi.LoginRequest{Email: "a@example.test", MasterPassword: password},
		TOTP:   func(context.Context) (string, error) { return "123456", nil },
		Output: "vault.kdbx", Credentials: kpdb.Credentials{Password: dbPassword}, Options: kpdb.DefaultOptions(), Progress: progress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.logins != 2 || client.lastTOTP != "123456" {
		t.Fatalf("login calls = %d, TOTP = %q", client.logins, client.lastTOTP)
	}
	if report.Attachments != 1 || !writer.called {
		t.Fatalf("report=%+v writer=%v", report, writer.called)
	}
	if len(progress.updates) < 5 {
		t.Fatalf("progress updates = %+v", progress.updates)
	}
	last := progress.updates[len(progress.updates)-1]
	if last.Stage != 3 || last.Completed != last.Total || last.Total != 1 {
		t.Fatalf("final progress = %+v", last)
	}
	foundConversionComplete := false
	foundSync := false
	foundWriting := false
	foundDownloaded := false
	for _, update := range progress.updates {
		if update.Stage == 1 && update.Indeterminate {
			foundSync = true
		}
		if update.Stage == 1 && update.Completed == 2 && update.Total == 2 {
			foundDownloaded = true
		}
		if update.Stage == 2 && update.Completed == 1 && update.Total == 1 {
			foundConversionComplete = true
		}
		if update.Stage == 3 && update.Indeterminate {
			foundWriting = true
		}
	}
	if !foundDownloaded {
		t.Fatalf("attachment completion missing from %+v", progress.updates)
	}
	if !foundSync || !foundConversionComplete || !foundWriting {
		t.Fatalf("sync, conversion, or writing progress missing from %+v", progress.updates)
	}
	if !allZero(password) || !allZero(dbPassword) {
		t.Fatal("credentials were not cleared")
	}
}

func TestExportStopsOnAttachmentFailure(t *testing.T) {
	session := &fakeSession{
		vault: bw.Vault{Items: []bw.Item{{
			ID: "item", Type: bw.ItemTypeSecureNote,
			Attachments: []bw.Attachment{{ID: "bad"}},
		}}},
		downloadErr: errors.New("network"),
	}
	writer := &fakeWriter{}
	_, err := New(&fakeClient{session: session, authenticated: true}, convert.New(), writer).Export(t.Context(), Request{
		Login: bwapi.LoginRequest{MasterPassword: []byte("master")}, Output: "x", Credentials: kpdb.Credentials{Password: []byte("db")}, Options: kpdb.DefaultOptions(),
	})
	if err == nil || writer.called {
		t.Fatalf("error=%v writer=%v", err, writer.called)
	}
}

type fakeClient struct {
	session       *fakeSession
	logins        int
	lastTOTP      string
	authenticated bool
}

func (c *fakeClient) Login(_ context.Context, request bwapi.LoginRequest) (bwapi.Session, error) {
	c.logins++
	c.lastTOTP = request.TOTP
	if !c.authenticated && request.TOTP == "" {
		return nil, &bwapi.TwoFactorRequiredError{Providers: []string{"authenticator"}}
	}
	return c.session, nil
}

type fakeSession struct {
	vault       bw.Vault
	downloadErr error
}

func (s *fakeSession) Sync(context.Context) (bw.Vault, error) { return s.vault, nil }
func (s *fakeSession) DownloadAttachment(context.Context, bw.Item, bw.Attachment) (io.ReadCloser, error) {
	if s.downloadErr != nil {
		return nil, s.downloadErr
	}
	return io.NopCloser(bytes.NewReader([]byte("attachment"))), nil
}
func (s *fakeSession) Close() error { return nil }

type fakeWriter struct{ called bool }

func (w *fakeWriter) WriteFile(context.Context, string, kp.Database, kpdb.Credentials, kpdb.Options, bool) error {
	w.called = true
	return nil
}

type fakeProgress struct{ updates []ProgressUpdate }

func (p *fakeProgress) Update(update ProgressUpdate) {
	p.updates = append(p.updates, update)
}

func allZero(values []byte) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}
	return true
}
