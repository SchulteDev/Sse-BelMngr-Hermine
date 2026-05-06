package hermine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	insertBmDocAssetQuery               = "INSERT OR IGNORE INTO " + tableNameAsset + " (uuid, name, docType, deleteState, docDate, timestampCreated, sync, needUpSync, needDownSync, targetDocType, ocrState, internalPath, fileSyncState) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)"
	selectBmDocAssetByIDQuery           = "SELECT * FROM " + tableNameAsset + " WHERE id = ?"
	selectBmDocAssetByInternalPathQuery = "SELECT * FROM " + tableNameAsset + " WHERE internalPath = ?"

	insertBmDocBelegQuery       = "INSERT OR IGNORE INTO " + tableNameBeleg + " (uuid, name, docType, deleteState, docDate, timestampCreated, sync, needUpSync, needDownSync, number, amount, netto, vat, comment, belegDate) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
	updateBmDocBelegQuery       = "UPDATE " + tableNameBeleg + " SET name = ?, docDate = ?, number = ?, amount = ?, netto = ?, vat = ?, comment = ?, belegDate = ? WHERE id = ?"
	selectBmDocBelegByUUIDQuery = "SELECT * FROM " + tableNameBeleg + " WHERE uuid = ?"
	selectBmDocBelegByIDQuery   = "SELECT * FROM " + tableNameBeleg + " WHERE id = ?"

	insertBmDocCategoryQuery       = "INSERT OR IGNORE INTO " + tableNameCategory + " (uuid, name, docType, deleteState, docDate, timestampCreated, sync, needUpSync, needDownSync) VALUES (?,?,?,?,?,?,?,?,?)"
	selectBmDocCategoryByNameQuery = "SELECT * FROM " + tableNameCategory + " WHERE name = ?"

	insertOrIgnoreBmDocLinkTableQuery     = "INSERT OR IGNORE INTO " + tableNameLinkTable + " (sourceUuid, targetUuid) VALUES (?,?)"
	selectBmDocLinkTableBySourceUUIDQuery = "SELECT * FROM " + tableNameLinkTable + " WHERE sourceUuid = ?"
	selectBmDocLinkTableByTargetUUIDQuery = "SELECT * FROM " + tableNameLinkTable + " WHERE targetUuid = ?"
)

type repository struct {
	q SqlxExecutor
}

func newRepository(q SqlxExecutor) *repository {
	return &repository{q: q}
}

func (r *repository) createIgnoreLink(logger *log.Entry, sourceUUID, targetUUID string) error {
	result := make([]*bmDocLink, 0)
	selectQuery := "SELECT * FROM " + tableNameLinkTable + " WHERE sourceUuid = ? AND targetUuid = ?"
	if err := r.q.Select(&result, selectQuery, sourceUUID, targetUUID); err != nil {
		logger.WithError(err).Warnf("Error when searching " + tableNameLinkTable + " for source %s and target %s", sourceUUID, targetUUID)
		return err
	}
	if len(result) > 0 {
		return nil
	}

	if _, err := r.q.Exec(insertOrIgnoreBmDocLinkTableQuery, sourceUUID, targetUUID); err != nil {
		logger.WithError(err).Warnf("Error when linking %s and %s as " + tableNameLinkTable + "", sourceUUID, targetUUID)
		return err
	}

	return nil
}

func (r *repository) findLinkByBelegAsTarget(logger *log.Entry, beleg *bmDocBeleg) ([]bmDocLink, error) {
	belegUUID := beleg.UUID
	result := make([]bmDocLink, 0)

	if err := r.q.Select(&result, selectBmDocLinkTableByTargetUUIDQuery, belegUUID); err != nil {
		logger.WithError(err).Warnf("Error when searching " + tableNameLinkTable + " for beleg %s as target", belegUUID)
		return nil, err
	}

	return result, nil
}

func (r *repository) findAssets(logger *log.Entry, belegManagerDirectory *os.File, pathOfFileToImport string) ([]*bmDocAsset, os.FileInfo, error) {
	fileBaseName := filepath.Base(pathOfFileToImport)

	belegManagerFilePath := filepath.Join(belegManagerDirectory.Name(), fileBaseName)
	fileStatInfo, fileStatErr := os.Stat(belegManagerFilePath)
	if fileStatErr != nil && !os.IsNotExist(fileStatErr) {
		logger.WithError(fileStatErr).Warnf("Error checking for file %s ", belegManagerFilePath)
		return nil, nil, fileStatErr
	}

	bmDocAssets := make([]*bmDocAsset, 0)
	if selectAssetErr := r.q.Select(&bmDocAssets, selectBmDocAssetByInternalPathQuery, fileBaseName); selectAssetErr != nil {
		logger.WithError(selectAssetErr).Warnf("Error when searching " + tableNameAsset + "-internalPath: %s", fileBaseName)
		return nil, nil, selectAssetErr
	}

	return bmDocAssets, fileStatInfo, nil
}

func (r *repository) createCategory(logger *log.Entry, documentFromAnalysis diDocument, fieldName string) error {
	bmDocUUID := newBmDocUUID()
	categoryName := documentFromAnalysis.getContentFieldCommaSeperated(fieldName)
	now := time.Now().Format(bmDocRFC3339Milli)
	if _, err := r.q.Exec(insertBmDocCategoryQuery, bmDocUUID, categoryName, 1, 0, now, now, 1, 1, 0); err != nil {
		logger.WithError(err).Warnf("Error when inserting %s as new " + tableNameCategory + " '%s': %s", fieldName, categoryName, err)
		return err
	}
	return nil
}

func (r *repository) linkCategoryToBeleg(logger *log.Entry, analysedDocument diDocument, fieldName string, beleg *bmDocBeleg) error {
	cat, catErr := r.findOrCreateCategory(logger, analysedDocument, fieldName)
	if catErr != nil {
		return catErr
	}
	if createLinkErr := r.createIgnoreLink(logger, cat.UUID, beleg.UUID); createLinkErr != nil {
		return createLinkErr
	}

	return nil
}

func (r *repository) createBelegWithLinkedAsset(logger *log.Entry, belegManagerDirectory *os.File, pathOfFileToImport string, documentFromAnalysis diDocument) (*bmDocBeleg, error) {
	internalFileToImportPath, createCopyErr := copyFileIntoDirectoryIfTargetDoesNotExist(logger, pathOfFileToImport, belegManagerDirectory.Name())
	if createCopyErr != nil {
		return nil, createCopyErr
	}

	fileBaseName := filepath.Base(pathOfFileToImport)
	newAsset, createAssetErr := r.createAsset(logger, fileBaseName, internalFileToImportPath)
	if createAssetErr != nil {
		_ = removeFile(logger, internalFileToImportPath)
		return nil, createAssetErr
	}
	if newAsset == nil {
		newAssetNotFoundErr := fmt.Errorf("expected new " + tableNameAsset + "-internalPath for %s", pathOfFileToImport)
		logger.WithError(newAssetNotFoundErr).Warn()
		return nil, newAssetNotFoundErr
	}

	beleg, createBelegErr := r.createBeleg(logger, documentFromAnalysis)
	if createBelegErr != nil {
		return nil, createBelegErr
	}
	if beleg == nil {
		noDocumentFoundError := fmt.Errorf("no " + tableNameBeleg + " found for asset %d though expected", newAsset.ID)
		logger.WithError(noDocumentFoundError).Warn()
		return nil, noDocumentFoundError
	}

	if createLinkErr := r.createIgnoreLink(logger, newAsset.UUID, beleg.UUID); createLinkErr != nil {
		return nil, createLinkErr
	}

	logger.
		WithField(logFieldBelegID, beleg.ID).
		WithField(logFieldBelegName, beleg.Name).
		Info("New Beleg created")
	return beleg, nil
}

func (r *repository) createBeleg(logger *log.Entry, documentFromAnalysis diDocument) (*bmDocBeleg, error) {
	fields := documentFromAnalysis.Fields

	bmDocUUID := newBmDocUUID()
	invoiceID := fields[fieldInvoiceID].Content
	invoiceDate := fields[fieldInvoiceDate].ValueDate
	name := documentFromAnalysis.createInvoiceName()
	now := time.Now().Format(bmDocRFC3339Milli)
	vat := documentFromAnalysis.getVat()
	gross := documentFromAnalysis.getGross()
	comment := documentFromAnalysis.createComment()
	if _, insertErr := r.q.Exec(insertBmDocBelegQuery, bmDocUUID, name, 3, 0, now, now, 1, 1, 0, invoiceID, gross, 0, vat, comment, invoiceDate); insertErr != nil {
		logger.WithError(insertErr).Warnf("Error when inserting new " + tableNameBeleg + "")
		return nil, insertErr
	}

	return r.findBelegByUUID(logger, &bmDocUUID)
}

func (r *repository) updateBeleg(logger *log.Entry, documentFromAnalysis diDocument, existingAsset *bmDocAsset) (*bmDocBeleg, error) {
	beleg, findBelegErr := r.findBelegByAsset(logger, existingAsset)
	if findBelegErr != nil {
		return nil, findBelegErr
	}
	if beleg == nil {
		noDocumentFoundError := fmt.Errorf("no " + tableNameBeleg + " found for asset %s though expected", existingAsset.UUID)
		logger.WithError(noDocumentFoundError).Warn()
		return nil, noDocumentFoundError
	}
	belegLogger := logger.WithField(logFieldBelegID, beleg.ID).WithField(logFieldBelegName, beleg.Name)

	fields := documentFromAnalysis.Fields
	invoiceID := fields[fieldInvoiceID].Content
	invoiceDate := fields[fieldInvoiceDate].ValueDate
	name := documentFromAnalysis.createInvoiceName()
	now := time.Now().Format(bmDocRFC3339Milli)
	vat := documentFromAnalysis.getVat()
	gross := documentFromAnalysis.getGross()
	comment := documentFromAnalysis.createComment()
	if _, err := r.q.Exec(updateBmDocBelegQuery, name, now, invoiceID, gross, 0, vat, comment, invoiceDate, beleg.ID); err != nil {
		belegLogger.WithError(err).Warnf("Error when updating " + tableNameBeleg + " %d", beleg.ID)
		return nil, err
	}

	updatedBeleg, findBelegErr := r.findBelegByID(belegLogger, beleg.ID)
	if findBelegErr != nil {
		return nil, findBelegErr
	}
	if updatedBeleg == nil {
		noDocumentFoundError := fmt.Errorf("no " + tableNameBeleg + " found for id %d though expected", beleg.ID)
		belegLogger.WithError(noDocumentFoundError).Warn()
		return nil, noDocumentFoundError
	}

	belegLogger.Info("Beleg updated")
	return updatedBeleg, nil
}

func (r *repository) findBelegByID(logger *log.Entry, id uint32) (*bmDocBeleg, error) {
	doc := bmDocBeleg{}
	if err := r.q.Get(&doc, selectBmDocBelegByIDQuery, id); err != nil {
		logger.WithError(err).Warnf("Error when searching " + tableNameBeleg + " for id %d", id)
		return nil, err
	}

	return &doc, nil
}

func (r *repository) findBelegByUUID(logger *log.Entry, uuid *string) (*bmDocBeleg, error) {
	doc := bmDocBeleg{}
	if err := r.q.Get(&doc, selectBmDocBelegByUUIDQuery, uuid); err != nil {
		logger.WithError(err).Warnf("Error when searching " + tableNameBeleg + " for uuid %s", *uuid)
		return nil, err
	}

	return &doc, nil
}

func (r *repository) findBelegByAsset(logger *log.Entry, asset *bmDocAsset) (*bmDocBeleg, error) {
	link, findLinkErr := r.findLinkByAssetAsSource(logger, asset)
	if findLinkErr != nil {
		return nil, findLinkErr
	}
	if link == nil {
		logger.Debugf("No link found for bmDocAsset %s", asset.UUID)
		return nil, nil
	}

	return r.findBelegByUUID(logger, &link.TargetUUID)
}

func (r *repository) findLinkByAssetAsSource(logger *log.Entry, asset *bmDocAsset) (*bmDocLink, error) {
	assetUUID := asset.UUID
	result := make([]*bmDocLink, 0)
	if err := r.q.Select(&result, selectBmDocLinkTableBySourceUUIDQuery, assetUUID); err != nil {
		logger.WithError(err).Warnf("Error when searching " + tableNameLinkTable + " for asset %s as source", assetUUID)
		return nil, err
	}
	if len(result) > 1 {
		err := fmt.Errorf("" + tableNameLinkTable + " for asset %s as source exists more than once, check in BelegManager", assetUUID)
		logger.Warn(err)
		return nil, err
	}

	if len(result) == 1 {
		return result[0], nil
	}
	return nil, nil
}

func (r *repository) createAsset(logger *log.Entry, fileName, internalPath string) (*bmDocAsset, error) {
	bmDocUUID := newBmDocUUID()
	now := time.Now().Format(bmDocRFC3339Milli)
	result, execErr := r.q.Exec(insertBmDocAssetQuery, bmDocUUID, fileName, 4, 0, now, now, 1, 1, 0, 3, 0, internalPath, 2)
	if execErr != nil {
		logger.WithError(execErr).Warnf("Error when inserting %s/%s as new " + tableNameAsset + "", fileName, internalPath)
		return nil, execErr
	}

	newID, lastInsertIDErr := result.LastInsertId()
	if lastInsertIDErr != nil {
		logger.WithError(lastInsertIDErr).Warn("Error retrieving last inserted ID")
		return nil, lastInsertIDErr
	}
	logger.WithField("bmdoc_asset_id", newID).Debug("Created new " + tableNameAsset + "")

	newAsset, newAssetErr := r.findAssetByID(logger, newID)
	if newAssetErr != nil {
		logger.WithError(newAssetErr).Warnf("Error when searching " + tableNameAsset + " for id %d", newID)
		return nil, newAssetErr
	}

	return newAsset, nil
}

func (r *repository) findAssetByID(logger *log.Entry, id int64) (*bmDocAsset, error) {
	asset := bmDocAsset{}
	if err := r.q.Get(&asset, selectBmDocAssetByIDQuery, id); err != nil {
		logger.WithError(err).Warnf("Error when searching " + tableNameAsset + " for id %d", id)
		return nil, err
	}

	return &asset, nil
}

func (r *repository) findOrCreateCategory(logger *log.Entry, documentFromAnalysis diDocument, fieldName string) (*bmDocCategory, error) {
	cat, catErr := r.findCategoryFromAnalysis(logger, documentFromAnalysis, fieldName)
	if cat != nil || catErr != nil {
		return cat, catErr
	}

	if err := r.createCategory(logger, documentFromAnalysis, fieldName); err != nil {
		return nil, err
	}
	return r.findCategoryFromAnalysis(logger, documentFromAnalysis, fieldName)
}

func (r *repository) findCategoryFromAnalysis(logger *log.Entry, documentFromAnalysis diDocument, fieldName string) (*bmDocCategory, error) {
	categoryName := documentFromAnalysis.getContentFieldCommaSeperated(fieldName)
	return r.findCategoryByName(logger, categoryName)
}

func (r *repository) findCategoryByName(logger *log.Entry, categoryName string) (*bmDocCategory, error) {
	result := make([]*bmDocCategory, 0)
	if err := r.q.Select(&result, selectBmDocCategoryByNameQuery, categoryName); err != nil {
		logger.WithError(err).Warnf("Error when searching for %s " + tableNameCategory + ": %s", categoryName, err)
		return nil, err
	}
	if len(result) > 1 {
		err := fmt.Errorf("" + tableNameCategory + " %s exists more than once, check in BelegManager", categoryName)
		logger.Warn(err)
		return nil, err
	}
	if len(result) == 1 && *result[0].DeleteState != 0 {
		err := fmt.Errorf("" + tableNameCategory + " %s is deleted, check in BelegManager", categoryName)
		logger.Warn(err)
		return nil, err
	}

	if len(result) == 1 {
		return result[0], nil
	}
	return nil, nil
}

func (r *repository) createOrUpdateBeleg(logger *log.Entry, belegManagerDirectory *os.File, pathOfFileToImport string, analysedDocument diDocument) (*bmDocBeleg, error) {
	fileToImportStatInfo, fileStatErr := os.Stat(pathOfFileToImport)
	if fileStatErr != nil && !os.IsNotExist(fileStatErr) {
		logger.WithError(fileStatErr).Warnf("Error checking for file %s ", pathOfFileToImport)
		return nil, fileStatErr
	}

	bmDocAssets, fileInfoForAsset, findAssetErr := r.findAssets(logger, belegManagerDirectory, pathOfFileToImport)
	if findAssetErr != nil {
		return nil, findAssetErr
	}

	if len(bmDocAssets) == 1 && !bmDocAssets[0].isDeleted() && fileInfoForAsset != nil && fileToImportStatInfo.Size() == fileInfoForAsset.Size() {
		return r.updateBeleg(logger, analysedDocument, bmDocAssets[0])
	}

	return r.createBelegWithLinkedAsset(logger, belegManagerDirectory, pathOfFileToImport, analysedDocument)
}
