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
