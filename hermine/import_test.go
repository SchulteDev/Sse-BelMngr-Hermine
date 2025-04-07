package hermine

import (
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	documentAnalysisExampleResultFileName = "di_result.json"
	invoiceExampleFileName                = "Azure DI example english invoice.png"
	testDataDirectoryName                 = "testdata"
)

func Test_importIntoBelegManager(t *testing.T) {
	t.Parallel()

	// given
	dbSemaphore := make(chan struct{}, 1)
	testLogger, _ := newDebuggingNullLogger(t)
	testLoggerEntry := testLogger.WithField("test", t.Name())

	tempDirPath := t.TempDir()
	tempDir, openTempDirErr := os.Open(tempDirPath)
	require.NoError(t, openTempDirErr)
	t.Cleanup(func() {
		closeErr := tempDir.Close()
		require.NoError(t, closeErr)
	})

	database := openDatabaseFixture(t, testLoggerEntry)
	invoiceAbsFilePath, diAr := getDiResultFixture(t)

	// when
	importedBeleg, importErrInsert := importIntoBelegManager(testLoggerEntry, database, dbSemaphore, tempDir, invoiceAbsFilePath, diAr.AnalyzeResult.Documents[0])
	require.NoError(t, importErrInsert)

	// then
	createdBeleg := assertBelegCreated(t, testLoggerEntry, database, tempDir, importedBeleg, invoiceAbsFilePath)

	// when
	time.Sleep(1 * time.Second)
	reimportedBeleg, importErrUpdate := importIntoBelegManager(testLoggerEntry, database, dbSemaphore, tempDir, invoiceAbsFilePath, diAr.AnalyzeResult.Documents[0])
	require.NoError(t, importErrUpdate)

	// then
	assertBelegUpdate(t, testLoggerEntry, database, createdBeleg, reimportedBeleg)
}

func assertBelegCreated(t *testing.T, logger *log.Entry, db *sqlx.DB, belegManagerDirectory *os.File, importedBeleg *bmDocBeleg, pathOfFileToImport string) *bmDocBeleg {
	t.Helper()

	bmDocAssets, fileInfoForAsset, findAssetErr := findBmDocAssets(logger, db, belegManagerDirectory, pathOfFileToImport)
	require.NoError(t, findAssetErr)
	require.Len(t, bmDocAssets, 1)
	require.NotNil(t, fileInfoForAsset)

	docAsset := bmDocAssets[0]
	require.EqualValues(t, 1, docAsset.ID)
	assert.NotEmpty(t, docAsset.UUID)
	assert.Equal(t, invoiceExampleFileName, docAsset.Name)
	assert.EqualValues(t, 4, *docAsset.DocType)
	assert.EqualValues(t, 3, *docAsset.TargetDocType)
	assert.EqualValues(t, 0, *docAsset.OcrState)
	assert.Equal(t, invoiceExampleFileName, *docAsset.InternalPath)
	assert.EqualValues(t, 2, *docAsset.FileSyncState)
	assertDefaultBmDocEntity(t, docAsset.bmDocEntity)

	originalFileContent, readFileToImportErr := os.ReadFile(pathOfFileToImport)
	require.NoError(t, readFileToImportErr)
	finalBelegFile := filepath.Join(belegManagerDirectory.Name(), *docAsset.InternalPath)
	finalBelegFileContent, readFinalBelegFileErr := os.ReadFile(finalBelegFile)
	require.NoError(t, readFinalBelegFileErr)
	require.Len(t, originalFileContent, len(finalBelegFileContent), "The lengths of the files' contents should be equal")
	assert.Equal(t, originalFileContent, finalBelegFileContent, "The contents of the files should be equal")

	beleg, findBelegErr := findBmDocBelegByID(logger, db, 1)
	require.NoError(t, findBelegErr)
	require.EqualValues(t, 1, beleg.ID)
	require.Equal(t, importedBeleg, beleg)
	assert.NotEmpty(t, beleg.UUID)
	assert.Equal(t, "MICROSOFT AND CONTONSO PARTNERSHIP PR... from CONTOSO", beleg.Name)
	assert.EqualValues(t, 3, *beleg.DocType)
	assert.Equal(t, "654123", *beleg.Number)
	assert.InEpsilon(t, 118368, *beleg.Amount, 0)
	assert.EqualValues(t, 0, *beleg.Netto)
	assert.InEpsilon(t, 20.0, *beleg.VAT, 0)
	assert.Equal(t, "- MICROSOFT AND CONTONSO PARTNERSHIP PROMOTION VIDEO PO 99881234\n\nInvoiceTotal confidence: 0.95", *beleg.Comment)
	assert.Equal(t, "2023-01-15", *beleg.BelegDate)
	assertDefaultBmDocEntity(t, beleg.bmDocEntity)

	msCategory, findMsCategoryErr := findBmDocCategoryByName(logger, db, "MICROSOFT")
	require.NoError(t, findMsCategoryErr)
	require.EqualValues(t, 11, msCategory.ID)
	assert.NotEmpty(t, msCategory.UUID)
	assert.Equal(t, "MICROSOFT", msCategory.Name)
	assert.EqualValues(t, 1, *msCategory.DocType)
	assertDefaultBmDocEntity(t, msCategory.bmDocEntity)

	ctsCategory, findCtsCategoryErr := findBmDocCategoryByName(logger, db, "CONTOSO")
	require.NoError(t, findCtsCategoryErr)
	require.EqualValues(t, 12, ctsCategory.ID)
	assert.NotEmpty(t, ctsCategory.UUID)
	assert.Equal(t, "CONTOSO", ctsCategory.Name)
	assert.EqualValues(t, 1, *ctsCategory.DocType)
	assertDefaultBmDocEntity(t, ctsCategory.bmDocEntity)

	docLinks, findDocLinksErr := findBmDocLinkByBelegAsTarget(logger, db, beleg)
	require.NoError(t, findDocLinksErr)
	require.Len(t, docLinks, 3)
	require.EqualValues(t, 1, docLinks[0].ID)
	assert.Equal(t, docAsset.UUID, docLinks[0].SourceUUID)
	assert.EqualValues(t, beleg.UUID, docLinks[0].TargetUUID)
	require.EqualValues(t, 2, docLinks[1].ID)
	assert.EqualValues(t, msCategory.UUID, docLinks[1].SourceUUID)
	assert.Equal(t, beleg.UUID, docLinks[1].TargetUUID)
	require.EqualValues(t, 3, docLinks[2].ID)
	assert.Equal(t, ctsCategory.UUID, docLinks[2].SourceUUID)
	assert.EqualValues(t, beleg.UUID, docLinks[2].TargetUUID)

	return beleg
}

func assertBelegUpdate(t *testing.T, logger *log.Entry, db *sqlx.DB, belegBefore, reimportedBeleg *bmDocBeleg) {
	t.Helper()

	beleg, findBelegErr := findBmDocBelegByID(logger, db, 1)
	require.NoError(t, findBelegErr)
	require.EqualValues(t, 1, beleg.ID)
	require.Equal(t, reimportedBeleg, beleg)
	assertDefaultBmDocEntity(t, beleg.bmDocEntity)
	assert.NotEqual(t, belegBefore.DocDate, beleg.DocDate)
}

func assertDefaultBmDocEntity(t *testing.T, e bmDocEntity) {
	t.Helper()

	assert.EqualValues(t, 0, *e.DeleteState)
	assert.NotEmpty(t, *e.DocDate)
	assert.NotEmpty(t, *e.TimestampCreated)
	assert.Nil(t, e.Unread)
	assert.EqualValues(t, 1, *e.Sync)
	assert.EqualValues(t, 1, *e.NeedUpSync)
	assert.EqualValues(t, 0, *e.NeedDownSync)
	assert.Nil(t, e.TimestampLastSync)
}

func openDatabaseFixture(t *testing.T, logger *log.Entry) *sqlx.DB {
	t.Helper()

	originalDBPath := filepath.Join(testDataDirectoryName, belMngrEmptySqLiteDatabaseFileName)
	copiedDBPath := filepath.Join(testDataDirectoryName, fmt.Sprintf("copied_%s_%s", time.Now().Format(flatDateTime), belMngrEmptySqLiteDatabaseFileName))
	backupErr := copyFileToTargetIfTargetDoesNotExist(logger, originalDBPath, copiedDBPath)
	require.NoError(t, backupErr)

	dsn := "file:" + copiedDBPath
	sqliteDB := sqlx.MustOpen("sqlite", dsn)
	t.Cleanup(func() {
		dbCloseErr := sqliteDB.Close()
		assert.NoError(t, dbCloseErr)

		fileRemoveErr := os.Remove(copiedDBPath)
		assert.NoError(t, fileRemoveErr)
	})

	return sqliteDB
}

func getDiResultFixture(t *testing.T) (string, *diAnalysisStatus) {
	t.Helper()

	resultFilePath := filepath.Join(testDataDirectoryName, documentAnalysisExampleResultFileName)
	file, readErr := os.ReadFile(resultFilePath)
	require.NoError(t, readErr)

	var diAr diAnalysisStatus
	unmarshalErr := json.Unmarshal(file, &diAr)
	require.NoError(t, unmarshalErr)

	invoiceFilePath := filepath.Join(testDataDirectoryName, invoiceExampleFileName)
	return invoiceFilePath, &diAr
}
