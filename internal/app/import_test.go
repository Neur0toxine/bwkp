package app

import (
	"context"
	"slices"
	"testing"

	"github.com/Neur0toxine/bwkp/pkg/convert"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
)

func TestPlanImportMatchesMetadataThenNaturalKey(t *testing.T) {
	vault := bw.Vault{
		Folders: []bw.Folder{{ID: "folder", Name: "Work"}},
		Items: []bw.Item{
			{ID: "metadata", FolderID: "other", Type: bw.ItemTypeSecureNote, Name: "Old"},
			{ID: "uuid", Type: bw.ItemTypeCard, Name: "Moved", Fields: []bw.Field{{Name: "KP.UUID", Value: "source-uuid", Type: 1}}},
			{ID: "natural", FolderID: "folder", Type: bw.ItemTypeLogin, Name: "Login"},
		},
	}
	entries := []convert.ImportEntry{
		{SourceItemID: "metadata", Folder: "Work", Item: bw.Item{Type: bw.ItemTypeSecureNote, Name: "Edited"}},
		{SourceUUID: "source-uuid", Folder: "Work", Item: bw.Item{Type: bw.ItemTypeCard, Name: "Card"}},
		{Folder: "Work", Item: bw.Item{Type: bw.ItemTypeLogin, Name: "Login"}},
	}
	operations, folders, err := planImport(entries, vault, ConflictUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if folders["Work"] != "folder" || operations[0].matched == nil || operations[0].matched.ID != "metadata" || operations[1].matched == nil || operations[1].matched.ID != "uuid" || operations[2].matched == nil || operations[2].matched.ID != "natural" {
		t.Fatalf("operations=%+v folders=%+v", operations, folders)
	}
}

func TestPlanImportRejectsAmbiguousMatches(t *testing.T) {
	vault := bw.Vault{Items: []bw.Item{
		{ID: "a", Type: bw.ItemTypeSecureNote, Name: "Note"},
		{ID: "b", Type: bw.ItemTypeSecureNote, Name: "Note"},
	}}
	_, _, err := planImport([]convert.ImportEntry{{Item: bw.Item{Type: bw.ItemTypeSecureNote, Name: "Note"}}}, vault, ConflictUpdate)
	if err == nil {
		t.Fatal("ambiguous match was accepted")
	}
}

func TestExecuteImportConflictModes(t *testing.T) {
	entry := convert.ImportEntry{SourceUUID: "source", Folder: "Work", Item: bw.Item{
		Type: bw.ItemTypeSecureNote, Name: "Note",
		Attachments: []bw.Attachment{{FileName: "new.bin", Content: []byte("new")}},
	}}
	matched := bw.Item{ID: "old", Type: bw.ItemTypeSecureNote, Name: "Note", Attachments: []bw.Attachment{{ID: "old-attachment"}}}
	tests := []struct {
		mode ConflictMode
		want []string
	}{
		{ConflictSkip, nil},
		{ConflictDelete, []string{"folder:Work", "trash:old", "create:new", "upload:new:new.bin"}},
		{ConflictUpdate, []string{"folder:Work", "update:old", "delete-attachment:old:old-attachment", "upload:old:new.bin"}},
		{ConflictDuplicate, []string{"folder:Work", "create:new", "upload:new:new.bin"}},
	}
	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			session := &fakeImportSession{}
			operation := importOperation{entry: entry, mode: test.mode}
			if test.mode != ConflictDuplicate {
				operation.matched = &matched
			}
			var result ImportResult
			if err := executeImportOperation(t.Context(), session, operation, map[string]string{}, &result); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(session.calls, test.want) {
				t.Fatalf("calls=%v want=%v", session.calls, test.want)
			}
		})
	}
}

type fakeImportSession struct {
	fakeSession
	calls []string
}

func (s *fakeImportSession) CreateFolder(_ context.Context, name string) (bw.Folder, error) {
	s.calls = append(s.calls, "folder:"+name)
	return bw.Folder{ID: "folder-id", Name: name}, nil
}

func (s *fakeImportSession) CreateItem(_ context.Context, _ bw.Item, _ string) (string, error) {
	s.calls = append(s.calls, "create:new")
	return "new", nil
}

func (s *fakeImportSession) UpdateItem(_ context.Context, id string, _ bw.Item, _ string) error {
	s.calls = append(s.calls, "update:"+id)
	return nil
}

func (s *fakeImportSession) TrashItem(_ context.Context, id string) error {
	s.calls = append(s.calls, "trash:"+id)
	return nil
}

func (s *fakeImportSession) RestoreItem(_ context.Context, id string) error {
	s.calls = append(s.calls, "restore:"+id)
	return nil
}

func (s *fakeImportSession) ArchiveItem(_ context.Context, id string) error {
	s.calls = append(s.calls, "archive:"+id)
	return nil
}

func (s *fakeImportSession) UnarchiveItem(_ context.Context, id string) error {
	s.calls = append(s.calls, "unarchive:"+id)
	return nil
}

func (s *fakeImportSession) DeleteAttachment(_ context.Context, itemID, attachmentID string) error {
	s.calls = append(s.calls, "delete-attachment:"+itemID+":"+attachmentID)
	return nil
}

func (s *fakeImportSession) UploadAttachment(_ context.Context, itemID string, attachment bw.Attachment) error {
	s.calls = append(s.calls, "upload:"+itemID+":"+attachment.FileName)
	return nil
}
