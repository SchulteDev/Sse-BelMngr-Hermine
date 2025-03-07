package cli

import (
	"github.com/SchulteMarkus/sse-belmngr-hermine/hermine"
	"github.com/bmatcuk/doublestar/v4"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

var (
	logLevelCliArgument                                            string
	absolutePathOfBelegManagerSqLiteDB                             string
	belegManagerDirectoryCliArgument, filesToImportGlobCliArgument string
	diEndpointCliArgument, diKeyCliArgument                        string
)

func validateCliArguments(_ *cobra.Command, _ []string) error {
	absolutePathOfBelegManagerSqLiteDB =
		filepath.Join(belegManagerDirectoryCliArgument, hermine.BelMngrSqLiteDatabaseFileName)
	if _, err := os.Stat(absolutePathOfBelegManagerSqLiteDB); os.IsNotExist(err) {
		log.Errorf("BelegManager database file '%s' does not exist", absolutePathOfBelegManagerSqLiteDB)
		return err
	} else if err != nil {
		log.WithError(err).
			Errorf("Error while checking for the BelegManager database file '%s'", absolutePathOfBelegManagerSqLiteDB)
		return err
	}

	return nil
}

func run(_ *cobra.Command, _ []string) error {
	initLogging(logLevelCliArgument)

	filesToImport, globErr := doublestar.FilepathGlob(filesToImportGlobCliArgument)
	if globErr != nil {
		log.WithField("glob_pattern", filesToImportGlobCliArgument).
			WithError(globErr).
			Errorf("Failed to get files to import from glob pattern")
		return globErr
	}
	log.WithField("glob_pattern", filesToImportGlobCliArgument).
		Debugf("Found %d file(s) for glob pattern", len(filesToImport))
	if len(filesToImport) == 0 {
		log.Info("No files to import found")
		return nil
	}

	belegManagerDirectory, err := os.Open(belegManagerDirectoryCliArgument)
	if err != nil {
		log.Panicf("Failed to open file: %v", err)
	}
	defer func() {
		if closeDirErr := belegManagerDirectory.Close(); closeDirErr != nil {
			log.WithError(closeDirErr).Debugf("Failed to close: %v", belegManagerDirectory)
		}
	}()

	if bErr := hermine.BackupBelegManagerSqLiteDatabaseFile(absolutePathOfBelegManagerSqLiteDB); bErr != nil {
		return bErr
	}
	sqLiteDB := hermine.StartBelegManagerSQLiteDB(absolutePathOfBelegManagerSqLiteDB)
	defer hermine.CloseDB(sqLiteDB)

	hermine.ProcessFiles(sqLiteDB, diEndpointCliArgument, diKeyCliArgument, belegManagerDirectory, filesToImport)
	return nil
}
