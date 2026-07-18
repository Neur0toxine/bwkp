package kpdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Neur0toxine/bwkp/internal/native"
	"github.com/Neur0toxine/bwkp/pkg/dto/kp"
)

type Reader interface {
	ReadFile(context.Context, string, Credentials) (kp.Database, error)
}

type NativeReader struct{}

func NewNativeReader() *NativeReader { return &NativeReader{} }

func (r *NativeReader) ReadFile(ctx context.Context, path string, credentials Credentials) (kp.Database, error) {
	if path == "" {
		return kp.Database{}, errors.New("input path is required")
	}
	if len(credentials.Password) == 0 && len(credentials.KeyFile) == 0 {
		return kp.Database{}, errors.New("a database password or key file is required")
	}
	if err := ctx.Err(); err != nil {
		return kp.Database{}, err
	}
	// #nosec G117 -- credentials are intentionally serialized only into the in-memory native ABI request.
	credentialsJSON, err := json.Marshal(struct {
		Password []byte `json:"password,omitzero"`
		KeyFile  []byte `json:"keyFile,omitzero"`
	}{credentials.Password, credentials.KeyFile})
	if err != nil {
		return kp.Database{}, fmt.Errorf("encode credentials: %w", err)
	}
	defer clear(credentialsJSON)
	payload, err := native.ReadKDBX(path, credentialsJSON)
	if err != nil {
		return kp.Database{}, err
	}
	defer clear(payload)
	var database kp.Database
	if err := json.Unmarshal(payload, &database); err != nil {
		return kp.Database{}, fmt.Errorf("decode KDBX database: %w", err)
	}
	return database, nil
}
