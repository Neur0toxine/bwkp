package bwapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Neur0toxine/bwkp/internal/native"
	"github.com/Neur0toxine/bwkp/pkg/dto/bw"
)

type NativeClient struct{}

func NewNativeClient() *NativeClient { return &NativeClient{} }

func (c *NativeClient) Login(ctx context.Context, request LoginRequest) (Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode login request: %w", err)
	}
	result, err := native.Login(payload)
	clear(payload)
	if err != nil {
		return nil, err
	}
	if len(result.Challenge) > 0 {
		var challenge struct {
			Providers []string `json:"providers"`
		}
		if err := json.Unmarshal(result.Challenge, &challenge); err != nil {
			return nil, fmt.Errorf("decode two-factor challenge: %w", err)
		}
		return nil, &TwoFactorRequiredError{Providers: challenge.Providers}
	}
	if result.Handle == 0 {
		return nil, errors.New("native login returned no session")
	}
	return &nativeSession{handle: result.Handle}, nil
}

type nativeSession struct {
	mu     sync.Mutex
	handle native.Handle
}

func (s *nativeSession) Sync(ctx context.Context) (bw.Vault, error) {
	if err := ctx.Err(); err != nil {
		return bw.Vault{}, err
	}
	handle, err := s.currentHandle()
	if err != nil {
		return bw.Vault{}, err
	}
	payload, err := native.Sync(handle)
	if err != nil {
		return bw.Vault{}, err
	}
	defer clear(payload)
	var vault bw.Vault
	if err := json.Unmarshal(payload, &vault); err != nil {
		return bw.Vault{}, fmt.Errorf("decode vault snapshot: %w", err)
	}
	return vault, nil
}

func (s *nativeSession) DownloadAttachment(ctx context.Context, item bw.Item, attachment bw.Attachment) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handle, err := s.currentHandle()
	if err != nil {
		return nil, err
	}
	request, err := json.Marshal(struct {
		ItemID     string        `json:"itemId"`
		Attachment bw.Attachment `json:"attachment"`
	}{item.ID, attachment})
	if err != nil {
		return nil, err
	}
	content, err := native.DownloadAttachment(handle, request)
	clear(request)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (s *nativeSession) CreateFolder(ctx context.Context, name string) (bw.Folder, error) {
	payload, err := s.mutate(ctx, map[string]any{"action": "createFolder", "name": name})
	if err != nil {
		return bw.Folder{}, err
	}
	var folder bw.Folder
	if err := json.Unmarshal(payload, &folder); err != nil {
		return bw.Folder{}, fmt.Errorf("decode created folder: %w", err)
	}
	return folder, nil
}

func (s *nativeSession) CreateItem(ctx context.Context, item bw.Item, folderID string) (string, error) {
	payload, err := s.mutate(ctx, map[string]any{"action": "createItem", "item": item, "folderId": folderID})
	if err != nil {
		return "", err
	}
	return mutationID(payload)
}

func (s *nativeSession) UpdateItem(ctx context.Context, id string, item bw.Item, folderID string) error {
	_, err := s.mutate(ctx, map[string]any{"action": "updateItem", "id": id, "item": item, "folderId": folderID})
	return err
}

func (s *nativeSession) TrashItem(ctx context.Context, id string) error {
	_, err := s.mutate(ctx, map[string]any{"action": "trashItem", "id": id})
	return err
}

func (s *nativeSession) RestoreItem(ctx context.Context, id string) error {
	_, err := s.mutate(ctx, map[string]any{"action": "restoreItem", "id": id})
	return err
}

func (s *nativeSession) ArchiveItem(ctx context.Context, id string) error {
	_, err := s.mutate(ctx, map[string]any{"action": "archiveItem", "id": id})
	return err
}

func (s *nativeSession) UnarchiveItem(ctx context.Context, id string) error {
	_, err := s.mutate(ctx, map[string]any{"action": "unarchiveItem", "id": id})
	return err
}

func (s *nativeSession) DeleteAttachment(ctx context.Context, itemID, attachmentID string) error {
	_, err := s.mutate(ctx, map[string]any{"action": "deleteAttachment", "itemId": itemID, "attachmentId": attachmentID})
	return err
}

func (s *nativeSession) UploadAttachment(ctx context.Context, itemID string, attachment bw.Attachment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	handle, err := s.currentHandle()
	if err != nil {
		return err
	}
	request, err := json.Marshal(map[string]any{"itemId": itemID, "fileName": attachment.FileName})
	if err != nil {
		return err
	}
	defer clear(request)
	response, err := native.UploadAttachment(handle, request, attachment.Content)
	clear(response)
	return err
}

func (s *nativeSession) mutate(ctx context.Context, request any) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handle, err := s.currentHandle()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	return native.Mutate(handle, payload)
}

func mutationID(payload []byte) (string, error) {
	defer clear(payload)
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", fmt.Errorf("decode mutated item: %w", err)
	}
	if result.ID == "" {
		return "", errors.New("mutated item returned no ID")
	}
	return result.ID, nil
}

func (s *nativeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return nil
	}
	err := native.Close(s.handle)
	s.handle = 0
	return err
}

func (s *nativeSession) currentHandle() (native.Handle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == 0 {
		return 0, errors.New("session is closed")
	}
	return s.handle, nil
}
