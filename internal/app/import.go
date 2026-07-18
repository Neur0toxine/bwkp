package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Neur0toxine/bwkp/internal/security"
	"github.com/Neur0toxine/bwkp/pkg/bwapi"
	"github.com/Neur0toxine/bwkp/pkg/convert"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
	"github.com/Neur0toxine/bwkp/pkg/kpdb"
)

type ConflictMode string

const (
	ConflictSkip      ConflictMode = "skip"
	ConflictDelete    ConflictMode = "delete"
	ConflictDuplicate ConflictMode = "duplicate"
	ConflictUpdate    ConflictMode = "update"
)

func (m ConflictMode) Validate() error {
	if m != ConflictSkip && m != ConflictDelete && m != ConflictDuplicate && m != ConflictUpdate {
		return fmt.Errorf("unsupported conflict mode %q", m)
	}
	return nil
}

type DatabaseReader interface {
	ReadFile(context.Context, string, kpdb.Credentials) (kp.Database, error)
}

type DatabaseConverter interface {
	Convert(kp.Database) ([]convert.ImportEntry, convert.ImportReport, error)
}

type ImportRequest struct {
	Login       bwapi.LoginRequest
	TOTP        TOTPProvider
	Input       string
	Credentials kpdb.Credentials
	Conflict    ConflictMode
	Progress    ProgressReporter
}

type ImportResult struct {
	Entries     int
	Created     int
	Updated     int
	Replaced    int
	Duplicated  int
	Skipped     int
	Fallbacks   int
	Attachments int
	Warnings    []convert.ImportWarning
}

type Importer struct {
	client    bwapi.Client
	reader    DatabaseReader
	converter DatabaseConverter
}

func NewImporter(client bwapi.Client, reader DatabaseReader, converter DatabaseConverter) *Importer {
	return &Importer{client: client, reader: reader, converter: converter}
}

func (i *Importer) Import(ctx context.Context, request ImportRequest) (result ImportResult, err error) {
	if request.Input == "" {
		return result, errors.New("input path is required")
	}
	if err := request.Conflict.Validate(); err != nil {
		return result, err
	}
	defer security.Clear(request.Login.MasterPassword)
	defer security.Clear(request.Credentials.Password)

	reportProgress(request.Progress, ProgressUpdate{Stage: 1, Stages: 3, Description: "Reading encrypted database...", Indeterminate: true})
	database, err := i.reader.ReadFile(ctx, request.Input, request.Credentials)
	if err != nil {
		return result, fmt.Errorf("read KDBX: %w", err)
	}
	entries, conversionReport, err := i.converter.Convert(database)
	if err != nil {
		return result, err
	}
	result.Entries = conversionReport.Entries
	result.Fallbacks = conversionReport.Fallbacks
	result.Warnings = conversionReport.Warnings
	reportProgress(request.Progress, ProgressUpdate{Stage: 1, Stages: 3, Description: "Reading encrypted database...", Completed: 1, Total: 1})

	session, err := loginSession(ctx, i.client, request.Login, request.TOTP)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, session.Close()) }()
	importSession, ok := session.(bwapi.ImportSession)
	if !ok {
		return result, errors.New("bitwarden session does not support imports")
	}
	reportProgress(request.Progress, ProgressUpdate{Stage: 2, Stages: 3, Description: "Synchronizing destination vault...", Indeterminate: true})
	vault, err := session.Sync(ctx)
	if err != nil {
		return result, fmt.Errorf("sync vault: %w", err)
	}
	operations, folders, err := planImport(entries, vault, request.Conflict)
	if err != nil {
		return result, fmt.Errorf("plan import: %w", err)
	}
	reportProgress(request.Progress, ProgressUpdate{Stage: 2, Stages: 3, Description: "Synchronizing destination vault...", Completed: 1, Total: 1})

	total := max(len(operations), 1)
	for index, operation := range operations {
		reportProgress(request.Progress, ProgressUpdate{Stage: 3, Stages: 3, Description: "Importing entries...", Completed: index, Total: total})
		if err := executeImportOperation(ctx, importSession, operation, folders, &result); err != nil {
			return result, fmt.Errorf("import entry %q (%s) after %d completed entries: %w", operation.entry.Item.Name, operation.entry.SourceUUID, index, err)
		}
	}
	reportProgress(request.Progress, ProgressUpdate{Stage: 3, Stages: 3, Description: "Importing entries...", Completed: total, Total: total})
	return result, nil
}

func loginSession(ctx context.Context, client bwapi.Client, request bwapi.LoginRequest, totp TOTPProvider) (bwapi.Session, error) {
	session, err := client.Login(ctx, request)
	if challenge, ok := errors.AsType[*bwapi.TwoFactorRequiredError](err); ok {
		if totp == nil {
			return nil, challenge
		}
		code, promptErr := totp(ctx)
		if promptErr != nil {
			return nil, fmt.Errorf("read TOTP: %w", promptErr)
		}
		request.TOTP = code
		session, err = client.Login(ctx, request)
	}
	if err != nil {
		return nil, fmt.Errorf("log in: %w", err)
	}
	return session, nil
}

type importOperation struct {
	entry   convert.ImportEntry
	matched *bw.Item
	mode    ConflictMode
}

func planImport(entries []convert.ImportEntry, vault bw.Vault, mode ConflictMode) ([]importOperation, map[string]string, error) {
	folderIDs := make(map[string]string, len(vault.Folders))
	duplicateFolders := make(map[string]struct{})
	for _, folder := range vault.Folders {
		if _, exists := folderIDs[folder.Name]; exists {
			duplicateFolders[folder.Name] = struct{}{}
		}
		folderIDs[folder.Name] = folder.ID
	}
	folderNames := make(map[string]string, len(vault.Folders))
	for _, folder := range vault.Folders {
		folderNames[folder.ID] = folder.Name
	}
	operations := make([]importOperation, 0, len(entries))
	for _, entry := range entries {
		if _, ambiguous := duplicateFolders[entry.Folder]; ambiguous {
			return nil, nil, fmt.Errorf("destination contains multiple folders named %q", entry.Folder)
		}
		matches := matchImportEntry(entry, vault.Items, folderNames)
		if mode == ConflictDuplicate {
			matches = nil
		}
		if len(matches) > 1 {
			return nil, nil, fmt.Errorf("entry %q has %d destination matches", entry.Item.Name, len(matches))
		}
		var matched *bw.Item
		if len(matches) == 1 {
			matched = new(matches[0])
		}
		operations = append(operations, importOperation{entry: entry, matched: matched, mode: mode})
	}
	return operations, folderIDs, nil
}

func matchImportEntry(entry convert.ImportEntry, items []bw.Item, folderNames map[string]string) []bw.Item {
	if entry.SourceItemID != "" {
		if index := slices.IndexFunc(items, func(item bw.Item) bool {
			return item.OrganizationID == "" && item.ID == entry.SourceItemID
		}); index >= 0 {
			return []bw.Item{items[index]}
		}
	}
	if entry.SourceUUID != "" {
		matches := slices.Collect(func(yield func(bw.Item) bool) {
			for _, item := range items {
				if item.OrganizationID == "" && slices.ContainsFunc(item.Fields, func(field bw.Field) bool {
					return field.Name == "KP.UUID" && field.Value == entry.SourceUUID
				}) && !yield(item) {
					return
				}
			}
		})
		if len(matches) > 0 {
			return matches
		}
	}
	return slices.Collect(func(yield func(bw.Item) bool) {
		for _, item := range items {
			if item.OrganizationID == "" && item.Name == entry.Item.Name && item.Type == entry.Item.Type && folderNames[item.FolderID] == entry.Folder {
				if !yield(item) {
					return
				}
			}
		}
	})
}

func executeImportOperation(ctx context.Context, session bwapi.ImportSession, operation importOperation, folders map[string]string, result *ImportResult) error {
	if operation.matched != nil && operation.mode == ConflictSkip {
		result.Skipped++
		return nil
	}
	folderID, err := ensureImportFolder(ctx, session, operation.entry.Folder, folders)
	if err != nil {
		return err
	}
	if operation.matched == nil {
		id, err := session.CreateItem(ctx, operation.entry.Item, folderID)
		if err != nil {
			return err
		}
		if operation.mode == ConflictDuplicate {
			result.Duplicated++
		} else {
			result.Created++
		}
		return finishImportedItem(ctx, session, id, operation.entry, result)
	}
	matched := *operation.matched
	switch operation.mode {
	case ConflictDelete:
		if err := session.TrashItem(ctx, matched.ID); err != nil {
			return err
		}
		id, err := session.CreateItem(ctx, operation.entry.Item, folderID)
		if err != nil {
			return err
		}
		result.Replaced++
		return finishImportedItem(ctx, session, id, operation.entry, result)
	case ConflictUpdate:
		if matched.DeletedDate != nil {
			if err := session.RestoreItem(ctx, matched.ID); err != nil {
				return err
			}
		}
		if matched.ArchivedDate != nil && !operation.entry.Archived {
			if err := session.UnarchiveItem(ctx, matched.ID); err != nil {
				return err
			}
		}
		if err := session.UpdateItem(ctx, matched.ID, operation.entry.Item, folderID); err != nil {
			return err
		}
		for _, attachment := range matched.Attachments {
			if err := session.DeleteAttachment(ctx, matched.ID, attachment.ID); err != nil {
				return err
			}
		}
		result.Updated++
		return finishImportedItem(ctx, session, matched.ID, operation.entry, result)
	case ConflictSkip, ConflictDuplicate:
		return fmt.Errorf("unexpected matched conflict mode %q", operation.mode)
	default:
		return fmt.Errorf("unexpected conflict mode %q", operation.mode)
	}
}

func ensureImportFolder(ctx context.Context, session bwapi.ImportSession, name string, folders map[string]string) (string, error) {
	if name == "" {
		return "", nil
	}
	if id := folders[name]; id != "" {
		return id, nil
	}
	folder, err := session.CreateFolder(ctx, name)
	if err != nil {
		return "", fmt.Errorf("create folder %q: %w", name, err)
	}
	if folder.ID == "" {
		return "", fmt.Errorf("created folder %q returned no ID", name)
	}
	folders[name] = folder.ID
	return folder.ID, nil
}

func finishImportedItem(ctx context.Context, session bwapi.ImportSession, id string, entry convert.ImportEntry, result *ImportResult) error {
	for _, attachment := range entry.Item.Attachments {
		if err := session.UploadAttachment(ctx, id, attachment); err != nil {
			return fmt.Errorf("upload attachment %q: %w", attachment.FileName, err)
		}
		result.Attachments++
	}
	if entry.Archived {
		if err := session.ArchiveItem(ctx, id); err != nil {
			return err
		}
	}
	if entry.Trashed {
		if err := session.TrashItem(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
