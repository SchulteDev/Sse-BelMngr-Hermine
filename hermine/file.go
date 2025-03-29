package hermine

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"time"
)

func copyFileIntoDirectoryIfTargetDoesNotExist(logger *log.Entry, filePath, directoryPath string) (string, error) {
	originalFileBaseName := filepath.Base(filePath)
	fileName := originalFileBaseName
	targetFilePath := filepath.Join(directoryPath, fileName)
	copyErr := copyFileToTargetIfTargetDoesNotExist(logger, filePath, targetFilePath)
	if copyErr != nil && os.IsExist(copyErr) {
		fileExt := filepath.Ext(originalFileBaseName)
		fileBaseName := originalFileBaseName[:len(originalFileBaseName)-len(fileExt)]
		nowAsFlatDateTime := time.Now().Format(flatDateTime)
		fileNameWithTimeSuffix := fmt.Sprintf("%s_%s%s", fileBaseName, nowAsFlatDateTime, fileExt)
		targetFilePathWithTimeSuffix := filepath.Join(directoryPath, fileNameWithTimeSuffix)
		logger.WithError(copyErr).Debugf("File %s already exists, copying %s to %s", targetFilePath, filePath, targetFilePathWithTimeSuffix)
		if copyErr = copyFileToTargetIfTargetDoesNotExist(logger, filePath, targetFilePathWithTimeSuffix); copyErr == nil {
			logger.Debugf("Copied %s to %s", filePath, targetFilePathWithTimeSuffix)
			return fileNameWithTimeSuffix, nil
		}
	}
	if copyErr != nil {
		logger.WithError(copyErr).Warnf("Error copying file %s to directory %s", filePath, directoryPath)
		return "", copyErr
	}

	logger.Debugf("Copied %s to %s", filePath, targetFilePath)
	return fileName, nil
}

func copyFileToTargetIfTargetDoesNotExist(logger *log.Entry, filePath, targetPath string) error {
	copyLogger := logger.WithField("file_to_backup", filePath).WithField("target", targetPath)
	closeFile := func(f *os.File) {
		fileLogger := copyLogger.WithField("file", f.Name())
		fileBaseName := filepath.Base(f.Name())
		if closeErr := f.Close(); closeErr == nil {
			fileLogger.Tracef("Closed file %s", fileBaseName)
		} else {
			fileLogger.Warnf("Failed to close file %s: %v", fileBaseName, closeErr)
		}
	}

	copyLogger.Trace("Creating copy of file...")

	sourceFile, openSrcErr := os.Open(filePath)
	if openSrcErr != nil {
		copyLogger.WithError(openSrcErr).Warnf("Failed to open source file: %v", openSrcErr)
		return openSrcErr
	}
	defer closeFile(sourceFile)

	if _, targetStatErr := os.Stat(targetPath); targetStatErr == nil {
		copyLogger.Debug("Target file already exists")
		return os.ErrExist
	} else if !os.IsNotExist(targetStatErr) {
		copyLogger.WithError(targetStatErr).Warnf("Failed to stat target file")
		return targetStatErr
	}

	destinationFile, createErr := os.Create(targetPath)
	if createErr != nil {
		copyLogger.WithError(createErr).Warnf("Failed to create destination file: %v", createErr)
		return createErr
	}
	defer closeFile(destinationFile)

	_, copyErr := io.Copy(destinationFile, sourceFile)
	if copyErr != nil {
		copyLogger.WithError(copyErr).Warnf("Failed to copy file contents: %v", copyErr)
		return copyErr
	}
	if stat, statErr := os.Stat(filePath); statErr != nil {
		copyLogger.WithError(statErr).Warnf("Failed to stat file permissions: %v", statErr)
		return statErr
	} else if chmodErr := os.Chmod(targetPath, stat.Mode()); chmodErr != nil {
		copyLogger.WithError(chmodErr).Warnf("Failed to set file permissions: %v", chmodErr)
		return chmodErr
	}

	if syncErr := destinationFile.Sync(); syncErr != nil {
		copyLogger.WithError(syncErr).Errorf("Failed to sync destination file: %v", syncErr)
		return syncErr
	}

	copyLogger.Debug("File copied successfully")
	return nil
}

func removeFile(logger *log.Entry, filePath string) error {
	if err := os.Remove(filePath); err != nil {
		logger.WithError(err).Warnf("Failed to remove file %s", filePath)
		return err
	}

	logger.Debugf("Removed file %s", filePath)
	return nil
}
