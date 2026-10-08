package management

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"minecraft-hosting/internal/server"
)

func TestBackupDestructionRequiresExactConfirmationBeforeProcessing(t *testing.T) {
	id := uuid.NewString()
	for _, action := range []string{"restore", "delete"} {
		for _, confirmation := range []string{"", "true", "confirmed", uuid.NewString()} {
			// No database or Panel: an unconfirmed operation must stop before either
			// dependency can be used, even when the supplied backup UUID is valid.
			s := &Service{}
			_, err := s.ChangeBackup(context.Background(), uuid.New(), uuid.New(), uuid.New(), id, action, confirmation, "confirmation-test")
			if !errors.Is(err, server.ErrInvalid) {
				t.Errorf("%s accepted confirmation %q: %v", action, confirmation, err)
			}
		}
	}
}
