package hermine

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"time"
)

const (
	insertBmDocAssetQuery               = "INSERT OR IGNORE INTO BmDoc_Asset (uuid, name, docType, deleteState, docDate, timestampCreated, sync, needUpSync, needDownSync, targetDocType, ocrState, internalPath, fileSyncState) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)"
	selectBmDocAssetByIDQuery           = "SELECT * FROM BmDoc_Asset WHERE id = ?"
	selectBmDocAssetByInternalPathQuery = "SELECT * FROM BmDoc_Asset WHERE internalPath = ?"

	insertBmDocBelegQuery       = "INSERT OR IGNORE INTO BmDoc_Beleg (uuid, name, docType, deleteState, docDate, timestampCreated, sync, needUpSync, needDownSync, number, amount, netto, vat, comment, belegDate) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
	updateBmDocBelegQuery       = "UPDATE BmDoc_Beleg SET name = ?, docDate = ?, number = ?, amount = ?, netto = ?, vat = ?, comment = ?, belegDate = ? WHERE id = ?"
	selectBmDocBelegByUUIDQuery = "SELECT * FROM BmDoc_Beleg WHERE uuid = ?"
	selectBmDocBelegByIDQuery   = "SELECT * FROM BmDoc_Beleg WHERE id = ?"

	insertBmDocCategoryQuery       = "INSERT OR IGNORE INTO BmDoc_Kategorie (uuid, name, docType, deleteState, docDate, timestampCreated, sync, needUpSync, needDownSync) VALUES (?,?,?,?,?,?,?,?,?)"
	selectBmDocCategoryByNameQuery = "SELECT * FROM BmDoc_Kategorie WHERE name = ?"

	insertOrIgnoreBmDocLinkTableQuery     = "INSERT OR IGNORE INTO BmDoc_LinkTable (sourceUuid, targetUuid) VALUES (?,?)"
	selectBmDocLinkTableBySourceUUIDQuery = "SELECT * FROM BmDoc_LinkTable WHERE sourceUuid = ?"
	selectBmDocLinkTableByTargetUUIDQuery = "SELECT * FROM BmDoc_LinkTable WHERE targetUuid = ?"
)

func createBmDocBelegWithLinkedAsset(logger *log.Entry, tx *sqlx.Tx, belegManagerDirectory *os.File, pathOfFileToImport string, documentFromAnalysis diDocument) (*bmDocBeleg, error) {
	internalFileToImportPath, createCopyErr := copyFileIntoDirectoryIfTargetDoesNotExist(logger, pathOfFileToImport, belegManagerDirectory.Name())
	if createCopyErr != nil {
		return nil, createCopyErr
	}

	fileBaseName := filepath.Base(pathOfFileToImport)
	newAsset, createAssetErr := createBmDocAsset(logger, tx, fileBaseName, internalFileToImportPath)
	if createAssetErr != nil {
		_ = removeFile(logger, internalFileToImportPath)
		return nil, createAssetErr
	}
	if newAsset == nil {
		newAssetNotFoundErr := fmt.Errorf("expected new BmDoc_Asset-internalPath for %s", pathOfFileToImport)
		logger.WithError(newAssetNotFoundErr).Warn()
		return nil, newAssetNotFoundErr
	}

	beleg, createBelegErr := createBmDocBeleg(logger, tx, documentFromAnalysis)
	if createBelegErr != nil {
		return nil, createBelegErr
	}
	if beleg == nil {
		noDocumentFoundError := fmt.Errorf("no BmDoc_Beleg found for asset %d though expected", newAsset.ID)
		logger.WithError(noDocumentFoundError).Warn()
		return nil, noDocumentFoundError
	}

	if createLinkErr := createIgnoreBmDocLink(logger, tx, newAsset.UUID, beleg.UUID); createLinkErr != nil {
		return nil, createLinkErr
	}

	logger.
		WithField("beleg_id", beleg.ID).
		WithField("beleg_name", beleg.Name).
		Info("New Beleg created")
	return beleg, nil
}

func createBmDocBeleg(logger *log.Entry, tx *sqlx.Tx, documentFromAnalysis diDocument) (*bmDocBeleg, error) {
	fields := documentFromAnalysis.Fields

	bmDocUUID := newBmDocUUID()
	invoiceID := fields["InvoiceId"].Content
	invoiceDate := fields["InvoiceDate"].ValueDate
	name := documentFromAnalysis.createInvoiceName()
	now := time.Now().Format(bmDocRFC3339Milli)
	vat := documentFromAnalysis.getVat()
	gross := documentFromAnalysis.getGross()
	comment := documentFromAnalysis.createComment()
	if _, insertErr := tx.Exec(insertBmDocBelegQuery, bmDocUUID, name, 3, 0, now, now, 1, 1, 0, invoiceID, gross, 0, vat, comment, invoiceDate); insertErr != nil {
		logger.WithError(insertErr).Warnf("Error when inserting new BmDoc_Beleg")
		return nil, insertErr
	}

	return findBmDocBelegByUUID(logger, tx, &bmDocUUID)
}

func updateBmDocBeleg(logger *log.Entry, tx *sqlx.Tx, documentFromAnalysis diDocument, existingAsset *bmDocAsset) (*bmDocBeleg, error) {
	beleg, findBelegErr := findBmDocBelegByAsset(logger, tx, existingAsset)
	if findBelegErr != nil {
		return nil, findBelegErr
	}
	if beleg == nil {
		noDocumentFoundError := fmt.Errorf("no BmDoc_Beleg found for asset %s though expected", existingAsset.UUID)
		logger.WithError(noDocumentFoundError).Warn()
		return nil, noDocumentFoundError
	}
	belegLogger := logger.WithField("beleg_id", beleg.ID).WithField("beleg_name", beleg.Name)

	fields := documentFromAnalysis.Fields
	invoiceID := fields["InvoiceId"].Content
	invoiceDate := fields["InvoiceDate"].ValueDate
	name := documentFromAnalysis.createInvoiceName()
	now := time.Now().Format(bmDocRFC3339Milli)
	vat := documentFromAnalysis.getVat()
	gross := documentFromAnalysis.getGross()
	comment := documentFromAnalysis.createComment()
	if _, err := tx.Exec(updateBmDocBelegQuery, name, now, invoiceID, gross, 0, vat, comment, invoiceDate, beleg.ID); err != nil {
		belegLogger.WithError(err).Warnf("Error when updating BmDoc_Beleg %d", beleg.ID)
		return nil, err
	}

	updatedBeleg, findBelegErr := findBmDocBelegByID(belegLogger, tx, beleg.ID)
	if findBelegErr != nil {
		return nil, findBelegErr
	}
	if updatedBeleg == nil {
		noDocumentFoundError := fmt.Errorf("no BmDoc_Beleg found for id %d though expected", beleg.ID)
		belegLogger.WithError(noDocumentFoundError).Warn()
		return nil, noDocumentFoundError
	}

	belegLogger.Info("Beleg updated")
	return updatedBeleg, nil
}

func findBmDocBelegByID(logger *log.Entry, q sqlxGetter, id uint32) (*bmDocBeleg, error) {
	doc := bmDocBeleg{}
	if err := q.Get(&doc, selectBmDocBelegByIDQuery, id); err != nil {
		logger.WithError(err).Warnf("Error when searching BmDoc_Beleg for id %d", id)
		return nil, err
	}

	return &doc, nil
}

func findBmDocBelegByUUID(logger *log.Entry, tx *sqlx.Tx, uuid *string) (*bmDocBeleg, error) {
	doc := bmDocBeleg{}
	if err := tx.Get(&doc, selectBmDocBelegByUUIDQuery, uuid); err != nil {
		logger.WithError(err).Warnf("Error when searching BmDoc_Beleg for uuid %s", *uuid)
		return nil, err
	}

	return &doc, nil
}

func findBmDocBelegByAsset(logger *log.Entry, tx *sqlx.Tx, asset *bmDocAsset) (*bmDocBeleg, error) {
	link, findLinkErr := findBmDocLinkByAssetAsSource(logger, tx, asset)
	if findLinkErr != nil {
		return nil, findLinkErr
	}
	if link == nil {
		logger.Debugf("No link found for bmDocAsset %s", asset.UUID)
		return nil, nil
	}

	return findBmDocBelegByUUID(logger, tx, &link.TargetUUID)
}

func createIgnoreBmDocLink(logger *log.Entry, tx *sqlx.Tx, sourceUUID, targetUUID string) error {
	if _, err := tx.Exec(insertOrIgnoreBmDocLinkTableQuery, sourceUUID, targetUUID); err != nil {
		logger.WithError(err).Warnf("Error when linking %s and %s as BmDoc_LinkTable", sourceUUID, targetUUID)
		return err
	}

	return nil
}

func findBmDocLinkByAssetAsSource(logger *log.Entry, tx *sqlx.Tx, asset *bmDocAsset) (*bmDocLink, error) {
	assetUUID := asset.UUID
	result := make([]*bmDocLink, 0)
	if err := tx.Select(&result, selectBmDocLinkTableBySourceUUIDQuery, assetUUID); err != nil {
		logger.WithError(err).Warnf("Error when searching BmDoc_LinkTable for asset %s as source", assetUUID)
		return nil, err
	}
	if len(result) > 1 {
		err := fmt.Errorf("BmDoc_LinkTable for asset %s as source exists more than once, check in BelegManager", assetUUID)
		logger.Warn(err)
		return nil, err
	}

	if len(result) == 1 {
		return result[0], nil
	}
	return nil, nil
}

func findBmDocLinkByBelegAsTarget(logger *log.Entry, q sqlxSelecter, beleg *bmDocBeleg) ([]bmDocLink, error) {
	belegUUID := beleg.UUID
	result := make([]bmDocLink, 0)

	if err := q.Select(&result, selectBmDocLinkTableByTargetUUIDQuery, belegUUID); err != nil {
		logger.WithError(err).Warnf("Error when searching BmDoc_LinkTable for beleg %s as target", belegUUID)
		return nil, err
	}

	return result, nil
}

func createBmDocAsset(logger *log.Entry, tx *sqlx.Tx, fileName, internalPath string) (*bmDocAsset, error) {
	bmDocUUID := newBmDocUUID()
	now := time.Now().Format(bmDocRFC3339Milli)
	result, execErr := tx.Exec(insertBmDocAssetQuery, bmDocUUID, fileName, 4, 0, now, now, 1, 1, 0, 3, 0, internalPath, 2)
	if execErr != nil {
		logger.WithError(execErr).Warnf("Error when inserting %s/%s as new BmDoc_Asset", fileName, internalPath)
		return nil, execErr
	}

	newID, lastInsertIDErr := result.LastInsertId()
	if lastInsertIDErr != nil {
		logger.WithError(lastInsertIDErr).Warn("Error retrieving last inserted ID")
		return nil, lastInsertIDErr
	}
	logger.WithField("bmdoc_asset_id", newID).Debug("Created new BmDoc_Asset")

	newAsset, newAssetErr := findBmDocAssetByID(logger, tx, newID)
	if newAssetErr != nil {
		logger.WithError(newAssetErr).Warnf("Error when searching BmDoc_Asset for id %d", newID)
		return nil, newAssetErr
	}

	return newAsset, nil
}

func findBmDocAssetByID(logger *log.Entry, q sqlxGetter, id int64) (*bmDocAsset, error) {
	asset := bmDocAsset{}
	if err := q.Get(&asset, selectBmDocAssetByIDQuery, id); err != nil {
		logger.WithError(err).Warnf("Error when searching BmDoc_Asset for id %d", id)
		return nil, err
	}

	return &asset, nil
}

func findBmDocAssets(logger *log.Entry, q sqlxSelecter, belegManagerDirectory *os.File, pathOfFileToImport string) ([]*bmDocAsset, os.FileInfo, error) {
	fileBaseName := filepath.Base(pathOfFileToImport)

	belegManagerFilePath := filepath.Join(belegManagerDirectory.Name(), fileBaseName)
	fileStatInfo, fileStatErr := os.Stat(belegManagerFilePath)
	if fileStatErr != nil && !os.IsNotExist(fileStatErr) {
		logger.WithError(fileStatErr).Warnf("Error checking for file %s ", belegManagerFilePath)
		return nil, nil, fileStatErr
	}

	bmDocAssets := make([]*bmDocAsset, 0)
	if selectAssetErr := q.Select(&bmDocAssets, selectBmDocAssetByInternalPathQuery, fileBaseName); selectAssetErr != nil {
		logger.WithError(selectAssetErr).Warnf("Error when searching BmDoc_Asset-internalPath: %s", fileBaseName)
		return nil, nil, selectAssetErr
	}

	return bmDocAssets, fileStatInfo, nil
}

func findOrCreateBmDocCategory(logger *log.Entry, tx *sqlx.Tx, documentFromAnalysis diDocument, fieldName string) (*bmDocCategory, error) {
	cat, catErr := findBmDocCategoryFromAnalysis(logger, tx, documentFromAnalysis, fieldName)
	if cat != nil || catErr != nil {
		return cat, catErr
	}

	if err := createBmDocCategory(logger, tx, documentFromAnalysis, fieldName); err != nil {
		return nil, err
	}
	return findBmDocCategoryFromAnalysis(logger, tx, documentFromAnalysis, fieldName)
}

func createBmDocCategory(logger *log.Entry, tx *sqlx.Tx, documentFromAnalysis diDocument, fieldName string) error {
	bmDocUUID := newBmDocUUID()
	categoryName := documentFromAnalysis.getContentFieldCommaSeperated(fieldName)
	now := time.Now().Format(bmDocRFC3339Milli)
	if _, err := tx.Exec(insertBmDocCategoryQuery, bmDocUUID, categoryName, 1, 0, now, now, 1, 1, 0); err != nil {
		logger.WithError(err).Warnf("Error when inserting %s as new BmDoc_Kategorie '%s': %s", fieldName, categoryName, err)
		return err
	}
	return nil
}

func findBmDocCategoryFromAnalysis(logger *log.Entry, tx *sqlx.Tx, documentFromAnalysis diDocument, fieldName string) (*bmDocCategory, error) {
	categoryName := documentFromAnalysis.getContentFieldCommaSeperated(fieldName)
	return findBmDocCategoryByName(logger, tx, categoryName)
}

func findBmDocCategoryByName(logger *log.Entry, q sqlxSelecter, categoryName string) (*bmDocCategory, error) {
	result := make([]*bmDocCategory, 0)
	if err := q.Select(&result, selectBmDocCategoryByNameQuery, categoryName); err != nil {
		logger.WithError(err).Warnf("Error when searching for %s BmDoc_Kategorie: %s", categoryName, err)
		return nil, err
	}
	if len(result) > 1 {
		err := fmt.Errorf("BmDoc_Kategorie %s exists more than once, check in BelegManager", categoryName)
		logger.Warn(err)
		return nil, err
	}
	if len(result) == 1 && *result[0].DeleteState != 0 {
		err := fmt.Errorf("BmDoc_Kategorie %s is deleted, check in BelegManager", categoryName)
		logger.Warn(err)
		return nil, err
	}

	if len(result) == 1 {
		return result[0], nil
	}
	return nil, nil
}
