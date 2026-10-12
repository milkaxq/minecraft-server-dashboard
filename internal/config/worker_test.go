package config

import "testing"

func TestBackupWorkerConfiguration(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://db/test?sslmode=disable")
	t.Setenv("PTERODACTYL_URL", "http://panel.example.com")
	t.Setenv("PTERODACTYL_APPLICATION_API_KEY", "application-test-key")
	t.Setenv("PTERODACTYL_INSTALLATION_API_KEY", "installation-test-key")
	t.Setenv("BACKUP_SCHEDULER_ENABLED", "")
	t.Setenv("PTERODACTYL_BACKUP_CLIENT_API_KEY", "")
	c, err := LoadWorker()
	if err != nil || c.BackupSchedulerEnabled {
		t.Fatalf("default enabled=%v error=%v", c.BackupSchedulerEnabled, err)
	}
	t.Setenv("BACKUP_SCHEDULER_ENABLED", "true")
	if _, err = LoadWorker(); err == nil {
		t.Fatal("enabled without explicit backup key")
	}
	t.Setenv("PTERODACTYL_BACKUP_CLIENT_API_KEY", "ptla_wrong-api-key")
	if _, err = LoadWorker(); err == nil {
		t.Fatal("Application key accepted as backup credential")
	}
	t.Setenv("PTERODACTYL_BACKUP_CLIENT_API_KEY", "test-backup-client-key")
	c, err = LoadWorker()
	if err != nil || !c.BackupSchedulerEnabled || c.BackupClientKey == "" {
		t.Fatal("explicit backup key rejected")
	}
	t.Setenv("BACKUP_SCHEDULER_ENABLED", "typo")
	if _, err = LoadWorker(); err == nil {
		t.Fatal("invalid scheduler flag accepted")
	}
}
