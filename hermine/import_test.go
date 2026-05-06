package hermine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	documentAnalysisExampleResultFileName = "di_result.json"
	invoiceExampleFileName                = "Azure DI example english invoice.png"
	testDataDirectoryName                 = "testdata"
)

const (
	testVendorMicrosoft = "MICROSOFT"
	testVendorContoso   = "CONTOSO"
	testInvoiceDate     = "2023-01-15"
)

func Test_importIntoBelegManager(t *testing.T) {
	t.Parallel()

	// given.
	dbSemaphore := make(chan struct{}, 1)
	testLogger, _ := newDebuggingNullLogger(t)
	testLoggerEntry := testLogger.WithField("test", t.Name())
	tempDir := t.TempDir()

	database := openDatabaseFixture(t, testLoggerEntry)
	invoiceAbsFilePath, diAr := getDiResultFixture(t)

	// when.
	importedBeleg, importErrInsert := importIntoBelegManager(testLoggerEntry, database, dbSemaphore, tempDir, invoiceAbsFilePath, diAr.AnalyzeResult.Documents[0])
	require.NoError(t, importErrInsert)

	// then.
	createdBeleg := assertBelegCreated(t, testLoggerEntry, database, tempDir, importedBeleg, invoiceAbsFilePath)

	// when.
	time.Sleep(1 * time.Second)
	reimportedBeleg, importErrUpdate := importIntoBelegManager(testLoggerEntry, database, dbSemaphore, tempDir, invoiceAbsFilePath, diAr.AnalyzeResult.Documents[0])
	require.NoError(t, importErrUpdate)

	// then.
	assertBelegUpdate(t, testLoggerEntry, database, createdBeleg, reimportedBeleg)
}

func assertBelegCreated(t *testing.T, logger *log.Entry, db *sqlx.DB, tempDir string, importedBeleg *bmDocBeleg, invoiceAbsFilePath string) *bmDocBeleg {
	t.Helper()

	repo := newRepository(db)
	beleg, findBelegErr := repo.findBelegByID(logger, 1)
	require.NoError(t, findBelegErr)
	require.EqualValues(t, 1, beleg.ID)
	assert.Equal(t, importedBeleg.UUID, beleg.UUID)
	assert.Equal(t, "Microsoft Contoso promotion from MICROSOFT to CONTOSO", beleg.Name)
	assert.EqualValues(t, 3, *beleg.DocType)
	assert.Equal(t, "654123", *beleg.Number)
	assert.InEpsilon(t, 118368, *beleg.Amount, 0)
	assert.EqualValues(t, 0, *beleg.Netto)
	assert.InEpsilon(t, 20.0, *beleg.VAT, 0)
	assert.Equal(t, "- MICROSOFT AND CONTONSO PARTNERSHIP PROMOTION VIDEO PO 99881234"+prefixInvoiceTotalConfidence+"0.95", *beleg.Comment)
	assert.Equal(t, testInvoiceDate, *beleg.BelegDate)
	assertDefaultBmDocEntity(t, beleg.bmDocEntity)

	msCategory, findMsCategoryErr := repo.findCategoryByName(logger, testVendorMicrosoft)
	require.NoError(t, findMsCategoryErr)
	require.EqualValues(t, 11, msCategory.ID)
	assert.NotEmpty(t, msCategory.UUID)
	assert.Equal(t, testVendorMicrosoft, msCategory.Name)
	assert.EqualValues(t, 1, *msCategory.DocType)
	assertDefaultBmDocEntity(t, msCategory.bmDocEntity)

	ctsCategory, findCtsCategoryErr := repo.findCategoryByName(logger, testVendorContoso)
	require.NoError(t, findCtsCategoryErr)
	require.EqualValues(t, 12, ctsCategory.ID)
	assert.NotEmpty(t, ctsCategory.UUID)
	assert.Equal(t, testVendorContoso, ctsCategory.Name)
	assert.EqualValues(t, 1, *ctsCategory.DocType)
	assertDefaultBmDocEntity(t, ctsCategory.bmDocEntity)

	docLinks, findDocLinksErr := repo.findLinkByBelegAsTarget(logger, beleg)
	require.NoError(t, findDocLinksErr)
	require.Len(t, docLinks, 3)
	foundAssetLink := false
	foundMsLink := false
	foundCtsLink := false
	for _, link := range docLinks {
		switch link.SourceUUID {
		case "{35805562-b91c-4384-904d-e99d3752e505}":
			foundAssetLink = true
		case msCategory.UUID:
			foundMsLink = true
		case ctsCategory.UUID:
			foundCtsLink = true
		}
	}
	assert.True(t, foundAssetLink, "Asset link not found")
	assert.True(t, foundMsLink, "Microsoft category link not found")
	assert.True(t, foundCtsLink, "Contoso category link not found")

	return beleg
}

func assertBelegUpdate(t *testing.T, logger *log.Entry, db *sqlx.DB, belegBefore, reimportedBeleg *bmDocBeleg) {
	t.Helper()

	repo := newRepository(db)
	beleg, findBelegErr := repo.findBelegByID(logger, 1)
	require.NoError(t, findBelegErr)
	require.EqualValues(t, 1, beleg.ID)
	assert.Equal(t, belegBefore.UUID, beleg.UUID)
	assert.Equal(t, reimportedBeleg.UUID, beleg.UUID)
	assert.Equal(t, "Microsoft Contoso promotion from MICROSOFT to CONTOSO", beleg.Name)

	assert.NotEqual(t, *belegBefore.TimestampCreated, *beleg.TimestampCreated)
	assert.Equal(t, *reimportedBeleg.TimestampCreated, *beleg.TimestampCreated)
}

func openDatabaseFixture(t *testing.T, logger *log.Entry) *sqlx.DB {
	t.Helper()
	belegManagerSqLiteDBAbsoluteFilePath := filepath.Join("testdata", belMngrEmptySqLiteDatabaseFileName)
	db := StartBelegManagerSQLiteDB(belegManagerSqLiteDBAbsoluteFilePath)
	return db
}

func getDiResultFixture(t *testing.T) (string, diAnalysisResult) {
	t.Helper()
	invoiceAbsFilePath, _ := filepath.Abs(filepath.Join(testDataDirectoryName, invoiceExampleFileName))
	diResultFilePath, _ := filepath.Abs(filepath.Join(testDataDirectoryName, documentAnalysisExampleResultFileName))
	diResultFileContent, _ := os.ReadFile(diResultFilePath)
	var diAr diAnalysisResult
	_ = json.Unmarshal(diResultFileContent, &diAr)
	return invoiceAbsFilePath, diAr
}

func assertDefaultBmDocEntity(t *testing.T, e bmDocEntity) {
	t.Helper()
	assert.NotEmpty(t, e.UUID)
	assert.NotEmpty(t, e.TimestampCreated)
	assert.EqualValues(t, 0, *e.DeleteState)
	assert.EqualValues(t, 1, *e.Sync)
	assert.EqualValues(t, 1, *e.NeedUpSync)
	assert.EqualValues(t, 0, *e.NeedDownSync)
}
