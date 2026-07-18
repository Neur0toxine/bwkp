package kpdb

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Neur0toxine/bitwarden-keepass-exporter/internal/atomicfile"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/internal/native"
	"github.com/Neur0toxine/bitwarden-keepass-exporter/pkg/dto/kp"
)

type NativeWriter struct{}

func NewNativeWriter() *NativeWriter { return &NativeWriter{} }

func (w *NativeWriter) WriteFile(ctx context.Context, target string, database kp.Database, credentials Credentials, options Options, force bool) error {
	if err := options.Validate(credentials); err != nil {
		return fmt.Errorf("database options: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	databaseJSON, err := json.Marshal(database)
	if err != nil {
		return fmt.Errorf("encode database: %w", err)
	}
	defer clear(databaseJSON)
	credentialsJSON, err := json.Marshal(struct {
		Password []byte `json:"password,omitzero"`
		KeyFile  []byte `json:"keyFile,omitzero"`
	}{credentials.Password, credentials.KeyFile})
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	defer clear(credentialsJSON)
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("encode options: %w", err)
	}
	return atomicfile.Write(target, force,
		func(candidate string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return native.WriteKDBX(candidate, databaseJSON, credentialsJSON, optionsJSON)
		},
		func(candidate string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return native.VerifyKDBX(candidate, credentialsJSON)
		},
	)
}
