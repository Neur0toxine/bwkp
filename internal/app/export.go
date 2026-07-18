package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Neur0toxine/bwkp/internal/security"
	"github.com/Neur0toxine/bwkp/pkg/bwapi"
	"github.com/Neur0toxine/bwkp/pkg/convert"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
	"github.com/Neur0toxine/bwkp/pkg/kpdb"
)

type VaultConverter interface {
	Convert(bw.Vault) (kp.Database, convert.Report, error)
}

type TOTPProvider func(context.Context) (string, error)

type ProgressUpdate struct {
	Stage         int
	Stages        int
	Description   string
	Completed     int
	Total         int
	Indeterminate bool
}

type ProgressReporter interface {
	Update(ProgressUpdate)
}

type Request struct {
	Login       bwapi.LoginRequest
	TOTP        TOTPProvider
	Output      string
	Force       bool
	Credentials kpdb.Credentials
	Options     kpdb.Options
	Progress    ProgressReporter
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

	reportProgress(request.Progress, ProgressUpdate{
		Stage: 1, Stages: 3, Description: "Downloading vault...", Indeterminate: true,
	})
	vault, err := session.Sync(ctx)
	if err != nil {
		return report, fmt.Errorf("sync vault: %w", err)
	}
	attachmentCount := countAttachments(vault)
	reportProgress(request.Progress, ProgressUpdate{
		Stage: 1, Stages: 3, Description: "Downloading vault...", Completed: 1, Total: 1 + attachmentCount,
	})
	if err := downloadAttachments(ctx, session, &vault, func(completed int) {
		reportProgress(request.Progress, ProgressUpdate{
			Stage: 1, Stages: 3, Description: "Downloading vault...", Completed: 1 + completed, Total: 1 + attachmentCount,
		})
	}); err != nil {
		return report, err
	}
	conversionTotal := max(len(vault.Items), 1)
	reportProgress(request.Progress, ProgressUpdate{Stage: 2, Stages: 3, Description: "Converting entries...", Total: conversionTotal})
	conversionCompleted := 0
	database, report, err := convertVault(e.converter, vault, func(completed int) {
		conversionCompleted = completed
		reportProgress(request.Progress, ProgressUpdate{
			Stage: 2, Stages: 3, Description: "Converting entries...", Completed: completed, Total: conversionTotal,
		})
	})
	if err != nil {
		return convert.Report{}, err
	}
	if conversionCompleted < conversionTotal {
		reportProgress(request.Progress, ProgressUpdate{
			Stage: 2, Stages: 3, Description: "Converting entries...", Completed: conversionTotal, Total: conversionTotal,
		})
	}
	reportProgress(request.Progress, ProgressUpdate{
		Stage: 3, Stages: 3, Description: "Writing encrypted database...", Indeterminate: true,
	})
	if err := e.writer.WriteFile(ctx, request.Output, database, request.Credentials, request.Options, request.Force); err != nil {
		return convert.Report{}, fmt.Errorf("write KDBX: %w", err)
	}
	reportProgress(request.Progress, ProgressUpdate{
		Stage: 3, Stages: 3, Description: "Writing encrypted database...", Completed: 1, Total: 1,
	})
	return report, nil
}

func downloadAttachments(ctx context.Context, session bwapi.Session, vault *bw.Vault, completed func(int)) error {
	downloaded := 0
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
			downloaded++
			completed(downloaded)
		}
	}
	return nil
}

func countAttachments(vault bw.Vault) int {
	total := 0
	for _, item := range vault.Items {
		total += len(item.Attachments)
	}
	return total
}

type progressiveVaultConverter interface {
	ConvertWithProgress(bw.Vault, func(completed, total int)) (kp.Database, convert.Report, error)
}

func convertVault(converter VaultConverter, vault bw.Vault, completed func(int)) (kp.Database, convert.Report, error) {
	if converter, ok := converter.(progressiveVaultConverter); ok {
		return converter.ConvertWithProgress(vault, func(completedItems, _ int) { completed(completedItems) })
	}
	database, report, err := converter.Convert(vault)
	if err == nil {
		completed(len(vault.Items))
	}
	return database, report, err
}

func reportProgress(reporter ProgressReporter, update ProgressUpdate) {
	if reporter != nil {
		reporter.Update(update)
	}
}
