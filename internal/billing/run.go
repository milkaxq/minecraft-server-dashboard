package billing

import (
	"context"
	"log/slog"
	"time"
)

// API and worker replicas can both run this loop; PostgreSQL arbitrates all
// entitlement/lifecycle decisions. No process-local scheduler lease is needed.
func (s *Service) RunReconciler(ctx context.Context, log *slog.Logger) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		task, cancel := context.WithTimeout(ctx, 30*time.Second)
		count, err := s.Reconcile(task)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Error("subscription reconciliation failed", "category", "database")
		} else if count > 0 {
			log.Info("subscriptions reconciled", "count", count)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
