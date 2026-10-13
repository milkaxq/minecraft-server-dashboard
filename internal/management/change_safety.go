package management

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrBackupUnavailable = errors.New("backup availability could not be checked")

var ErrBackupDecision = errors.New("choose a completed backup or explicitly acknowledge proceeding without one")

type BackupDecision struct {
	Backup        *uuid.UUID `json:"backup"`
	WithoutBackup bool       `json:"acknowledge_no_backup"`
}

func (s *Service) checkBackupDecision(ctx context.Context, t Target, in BackupDecision) error {
	if in.WithoutBackup && in.Backup == nil {
		return nil
	}
	if in.WithoutBackup || in.Backup == nil || *in.Backup == uuid.Nil {
		return ErrBackupDecision
	}
	list, err := s.Panel.ListBackups(ctx, t.Identifier)
	if err != nil {
		return ErrBackupUnavailable
	}
	backup, err := backupIn(list, in.Backup.String())
	if err != nil {
		return err
	}
	if !backup.Successful || backup.CompletedAt == nil || *backup.CompletedAt == "" {
		return ErrBackupDecision
	}
	if _, err := time.Parse(time.RFC3339, *backup.CompletedAt); err != nil {
		return ErrBackupDecision
	}
	return nil
}
