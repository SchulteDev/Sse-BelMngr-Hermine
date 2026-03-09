package hermine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_copyFileIntoDirectoryIfTargetDoesNotExist_copyOneFile(t *testing.T) {
	testLogger, _ := newDebuggingNullLogger(t)
	testLoggerEntry := testLogger.WithField("test", t.Name())

	fileToCopy, createErr := os.CreateTemp(t.TempDir(), "testfile1.txt")
	require.NoError(t, createErr)
	t.Cleanup(func() {
		removeErr := os.Remove(fileToCopy.Name())
		assert.NoError(t, removeErr)
	})

	fileToCopyCloseErr := fileToCopy.Close()
	require.NoError(t, fileToCopyCloseErr)

	targetDirectory := t.TempDir()
	createdFile, createCopyErr := copyFileIntoDirectoryIfTargetDoesNotExist(testLoggerEntry, fileToCopy.Name(), targetDirectory)
	require.NoError(t, createCopyErr)
	require.NotNil(t, createdFile)

	dirEntries, readDirErr := os.ReadDir(targetDirectory)
	require.NoError(t, readDirErr)
	require.Len(t, dirEntries, 1)
	assert.Equal(t, filepath.Base(fileToCopy.Name()), dirEntries[0].Name())
}

func Test_copyFileIntoDirectoryIfTargetDoesNotExist_targetAlreadyPresent(t *testing.T) {
	testLogger, _ := newDebuggingNullLogger(t)
	testLoggerEntry := testLogger.WithField("test", t.Name())

	fileToCopy, createErr := os.CreateTemp(t.TempDir(), "testfile2.txt")
	require.NoError(t, createErr)
	fileToCopyName := fileToCopy.Name()
	fileToCopyBaseName := filepath.Base(fileToCopyName)
	t.Cleanup(func() {
		removeErr := os.Remove(fileToCopyName)
		assert.NoError(t, removeErr)
	})

	fileToCopyCloseErr := fileToCopy.Close()
	require.NoError(t, fileToCopyCloseErr)

	targetDirectory := t.TempDir()
	emptyFilePath := filepath.Join(targetDirectory, fileToCopyBaseName)
	emptyFile, createEmptyFileErr := os.Create(emptyFilePath)
	require.NoError(t, createEmptyFileErr)
	require.Equal(t, emptyFilePath, emptyFile.Name())
	t.Cleanup(func() {
		removeErr := os.Remove(emptyFile.Name())
		assert.NoError(t, removeErr)
	})

	emptyFileCloseErr := emptyFile.Close()
	require.NoError(t, emptyFileCloseErr)

	createdFile, createCopyErr := copyFileIntoDirectoryIfTargetDoesNotExist(testLoggerEntry, fileToCopyName, targetDirectory)
	require.NoError(t, createCopyErr)
	require.NotNil(t, createdFile)

	dirEntries, readDirErr := os.ReadDir(targetDirectory)
	require.NoError(t, readDirErr)
	require.Len(t, dirEntries, 2)
	assert.Equal(t, fileToCopyBaseName, dirEntries[0].Name())
}
