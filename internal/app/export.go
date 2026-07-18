package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Neur0toxine/bitwarden-keepass-exporter/internal/security"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/bwapi"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/convert"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/dto/bw"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/dto/kp"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/kpdb"
)

type VaultConverter interface {
	Convert(bw.Vault) (kp.Database, convert.Report, error)
}

type TOTPProvider func(context.Context) (string, error)

type Request struct {
	Login       bwapi.LoginRequest
	TOTP        TOTPProvider
	Output      string
	Force       bool
	Credentials kpdb.Credentials
	Options     kpdb.Options
}

type Exporter struct {
	client    bwapi.Client
	converter VaultConverter
	writer    kpdb.Writer
}

func New(client bwapi.Client, converter VaultConverter, writer kpdb.Writer) *Exporter {
	return &Exporter{client: client, converter: converter, writer: writer}
}

func (e *Exporter) Export(ctx context.Context, request Request) (report convert.Report, err error) {
	if request.Output == "" {
		return report, errors.New("output path is required")
	}
	defer security.Clear(request.Login.MasterPassword)
	defer security.Clear(request.Credentials.Password)

	session, err := e.client.Login(ctx, request.Login)
	if challenge, ok := errors.AsType[*bwapi.TwoFactorRequiredError](err); ok {
		if request.TOTP == nil {
			return report, challenge
		}
		code, promptErr := request.TOTP(ctx)
		if promptErr != nil {
			return report, fmt.Errorf("read TOTP: %w", promptErr)
		}
		request.Login.TOTP = code
		session, err = e.client.Login(ctx, request.Login)
	}
	if err != nil {
		return report, fmt.Errorf("log in: %w", err)
	}
	defer func() { err = errors.Join(err, session.Close()) }()

	vault, err := session.Sync(ctx)
	if err != nil {
		return report, fmt.Errorf("sync vault: %w", err)
	}
	if err := downloadAttachments(ctx, session, &vault); err != nil {
		return report, err
	}
	database, report, err := e.converter.Convert(vault)
	if err != nil {
		return convert.Report{}, err
	}
	if err := e.writer.WriteFile(ctx, request.Output, database, request.Credentials, request.Options, request.Force); err != nil {
		return convert.Report{}, fmt.Errorf("write KDBX: %w", err)
	}
	return report, nil
}

func downloadAttachments(ctx context.Context, session bwapi.Session, vault *bw.Vault) error {
	for itemIndex := range vault.Items {
		item := &vault.Items[itemIndex]
		for attachmentIndex := range item.Attachments {
			attachment := &item.Attachments[attachmentIndex]
			reader, err := session.DownloadAttachment(ctx, *item, *attachment)
			if err != nil {
				return fmt.Errorf("download attachment %s for item %s: %w", attachment.ID, item.ID, err)
			}
			content, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if err := errors.Join(readErr, closeErr); err != nil {
				return fmt.Errorf("read attachment %s for item %s: %w", attachment.ID, item.ID, err)
			}
			attachment.Content = content
		}
	}
	return nil
}
