package db

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"lodestar/internal/config"
	"lodestar/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		return nil, err
	}

	database, err := gorm.Open(sqlite.Open(cfg.DBPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{
		Logger: newLogger(cfg.SQLDebug),
	})
	if err != nil {
		return nil, err
	}

	if err := database.AutoMigrate(
		&models.User{},
		&models.Session{},
		&models.Project{},
		&models.Release{},
		&models.Package{},
		&models.Target{},
		&models.TargetApp{},
		&models.DeployJob{},
		&models.DeployLog{},
		&models.VersionPolicy{},
		&models.BlockedVersion{},
		&models.RequestNonce{},
		&models.AuditLog{},
		&models.Setting{},
	); err != nil {
		return nil, err
	}
	return database, nil
}

// newLogger keeps the console readable by default.
//
// GORM's stock logger logs every query at Info, and even at Warn it prints a
// line for each ErrRecordNotFound — which happens constantly here (looking up
// a session, a setting, a nonce). The result was a wall of SQL in the logs.
//
// Default behaviour is therefore: only real SQL errors, no slow-query noise,
// no "record not found". Set LODESTAR_SQL_DEBUG=1 (or sql_debug: true in
// system.yaml) to get the full statement log back.
func newLogger(debug bool) logger.Interface {
	if debug {
		return logger.New(
			log.New(os.Stdout, "[sql] ", log.LstdFlags),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  logger.Info,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		)
	}
	return logger.New(
		log.New(os.Stdout, "[sql] ", log.LstdFlags),
		logger.Config{
			// 0 disables slow-query reporting entirely.
			SlowThreshold:             0,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}
