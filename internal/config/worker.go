package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Worker configuration is separate from the API: only the worker needs the Application key.
type Worker struct {
	Environment, DatabaseURL, PanelURL, ApplicationKey, InstallationKey, BackupClientKey string
	BackupSchedulerEnabled                                                               bool
}

func LoadWorker() (Worker, error) {
	c := Worker{Environment: env("APP_ENV", "development"), DatabaseURL: os.Getenv("DATABASE_URL"), PanelURL: os.Getenv("PTERODACTYL_URL"), ApplicationKey: os.Getenv("PTERODACTYL_APPLICATION_API_KEY"), InstallationKey: os.Getenv("PTERODACTYL_INSTALLATION_API_KEY")}
	var err error
	c.BackupSchedulerEnabled, err = backupSchedulerFlag()
	if err != nil {
		return c, err
	}
	c.BackupClientKey = os.Getenv("PTERODACTYL_BACKUP_CLIENT_API_KEY")
	if c.BackupSchedulerEnabled && (c.BackupClientKey == "" || strings.HasPrefix(c.BackupClientKey, "ptla_")) {
		return c, fmt.Errorf("enabled backups require PTERODACTYL_BACKUP_CLIENT_API_KEY containing a Client API credential")
	}
	if c.Environment != "development" && c.Environment != "production" {
		return c, fmt.Errorf("invalid environment")
	}
	if c.DatabaseURL == "" || c.PanelURL == "" || c.ApplicationKey == "" {
		return c, fmt.Errorf("worker requires DATABASE_URL, PTERODACTYL_URL and PTERODACTYL_APPLICATION_API_KEY")
	}
	if c.Environment == "production" {
		if err := secureDatabase(c.DatabaseURL); err != nil {
			return c, err
		}
	}
	return c, nil
}

func OperatorDatabaseURL() (string, error) {
	environment := env("APP_ENV", "development")
	if environment != "development" && environment != "production" {
		return "", fmt.Errorf("invalid environment")
	}
	value := os.Getenv("DATABASE_URL")
	if value == "" {
		return "", fmt.Errorf("DATABASE_URL required")
	}
	if environment == "production" {
		if err := secureDatabase(value); err != nil {
			return "", err
		}
	}
	return value, nil
}

func backupSchedulerFlag() (bool, error) {
	enabled, err := strconv.ParseBool(env("BACKUP_SCHEDULER_ENABLED", "false"))
	if err != nil {
		return false, fmt.Errorf("BACKUP_SCHEDULER_ENABLED must be a boolean")
	}
	return enabled, nil
}
