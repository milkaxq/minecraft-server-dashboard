package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func Record(ctx context.Context, tx pgx.Tx, actor, server uuid.UUID, action, requestID string, metadata any) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,actor_id,server_id,action,request_id,metadata) VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), actor, nullableID(server), action, requestID, data)
	return err
}

func nullableID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
