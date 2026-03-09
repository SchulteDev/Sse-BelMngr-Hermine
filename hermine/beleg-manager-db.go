package hermine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	_ "modernc.org/sqlite" // Imported for usage with sqlx, which relies on a database driver
)

const (
	BelMngrSqLiteDatabaseFileEnding = ".db4"
	BelMngrSqLiteDatabaseFileName   = "BelegManager" + BelMngrSqLiteDatabaseFileEnding
	belMngrSupportedDBVersion       = 22003
	flatDateTime                    = "20060102150405"
	sseBelMngrTablesExistQuery      = "SELECT name FROM sqlite_master WHERE type = 'table' AND name IN ('BmDoc_Asset', 'BmDoc_Beleg', 'BmDoc_Kategorie', 'BmDoc_LinkTable', 'BmDoc_Person','BmDoc_Steuerfall')"
	sseBelMngrDBVersionQuery        = "SELECT propertyValue FROM Property WHERE propertyKey = 'databaseVersion'"
)

func StartBelegManagerSQLiteDB(dbFilePath string) *sqlx.DB {
	db := mustOpenSqLiteDB(dbFilePath)
	mustBeSupportedBelegManagerDBVersion(db)
	mustHavePresupposedTables(db)

	return db
}

func mustOpenSqLiteDB(dbFilePath string) *sqlx.DB {
	dbFileLogger := log.WithField("db_file", dbFilePath)

	dsn := fmt.Sprintf("file:%s?cache=shared", dbFilePath)
	sqliteDB := sqlx.MustOpen("sqlite", dsn)
	dbFileLogger.Debugf("Opened SQLite database")
	if pingErr := sqliteDB.Ping(); pingErr != nil {
		dbFileLogger.Panicf("Failed to connect to the database: %v", pingErr)
	}

	return sqliteDB
}

func mustBeSupportedBelegManagerDBVersion(db *sqlx.DB) {
	var v uint16
	if queryErr := db.QueryRow(sseBelMngrDBVersionQuery).Scan(&v); queryErr != nil {
		log.WithError(queryErr).Panic()
	}
	log.Tracef("BelegManager database schema version: %d", v)

	if v != belMngrSupportedDBVersion {
		log.Panicf("BelegManager database version %d not supported, supported: %d", v, belMngrSupportedDBVersion)
	}
}

func mustHavePresupposedTables(db *sqlx.DB) {
	var presupposedTables []string
	queryErr := db.Select(&presupposedTables, sseBelMngrTablesExistQuery)
	if queryErr != nil {
		log.WithError(queryErr).Panic()
	}
	if len(presupposedTables) != 6 {
		log.Panicf("SSE BelegManager tables not present, there are: %s", presupposedTables)
	}
}

func CloseDB(db *sqlx.DB) {
	if err := db.Close(); err != nil {
		log.WithError(err).Debug("Failed to close database")
	}
}

func BackupBelegManagerSqLiteDatabaseFile(dbFilePath string) error {
	dummyLogEntry := log.WithField("dummy", true)
	fpWithoutFileEnding := strings.TrimSuffix(dbFilePath, BelMngrSqLiteDatabaseFileEnding)
	backupPath := fmt.Sprintf("%s-schultedevbackup-%s.db4", fpWithoutFileEnding, time.Now().Format(flatDateTime))

	return copyFileToTargetIfTargetDoesNotExist(dummyLogEntry, dbFilePath, backupPath)
}

func beginTransaction(db *sqlx.DB) (*sqlx.Tx, error) {
	tx, beginTxErr := db.BeginTxx(context.Background(), nil)
	if beginTxErr != nil {
		log.WithError(beginTxErr).Warn("Failed to begin database transaction")
		return nil, beginTxErr
	}

	log.Debug("Transaction begun")
	return tx, nil
}

func finishTransaction(tx *sqlx.Tx) {
	p := recover()
	if p != nil {
		log.WithField("panic", p).Warn("Panic during transaction, rolling back")
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.WithError(rollbackErr).Warn("Failed to rollback transaction after panic")
		}
		panic(p)
	}

	if commitErr := tx.Commit(); commitErr == nil {
		log.Debug("Transaction committed")
	} else {
		log.WithError(commitErr).Warn("Failed to commit transaction")
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			log.WithError(rollbackErr).Warn("Failed to rollback transaction after commit error")
		}
	}
}

func newBmDocUUID() string {
	rawUUID := uuid.New()
	return fmt.Sprintf("{%s}", rawUUID)
}
