package hermine

import (
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"sync"
)

func ProcessFiles(db *sqlx.DB, diEndpoint, diKey string, belegManagerDirectory *os.File, filesToImport []string) {
	pdds := gatherResultsFromProcessingFiles(db, diEndpoint, diKey, belegManagerDirectory, filesToImport)
	logToCsv(belegManagerDirectory, pdds)
}

func gatherResultsFromProcessingFiles(db *sqlx.DB, diEndpoint, diKey string, belegManagerDirectory *os.File, filesToImport []string) []*processingDoneData {
	dbSemaphore := make(chan struct{}, 1)
	var wg sync.WaitGroup
	results := make(chan []*processingDoneData)
	for _, pathOfFileToImport := range filesToImport {
		wg.Add(1)

		go func(p string) {
			defer wg.Done()
			results <- processFile(db, dbSemaphore, diEndpoint, diKey, belegManagerDirectory, p)
		}(pathOfFileToImport)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	var pdds []*processingDoneData
	for r := range results {
		pdds = append(pdds, r...)
	}

	return pdds
}

func processFile(db *sqlx.DB, dbSemaphore chan struct{}, diEndpoint, diKey string, belegManagerDirectory *os.File, rawPathOfFileToImport string) []*processingDoneData {
	pathOfFileToImport := filepath.Clean(rawPathOfFileToImport)
	pathOfFileToImportBaseName := filepath.Base(pathOfFileToImport)
	fileLogger := log.
		WithField("file_to_import_base_name", pathOfFileToImportBaseName).
		WithField("file_to_import_full_path", pathOfFileToImport)
	fileLogger.Tracef("Processing %s...", pathOfFileToImportBaseName)

	analysisResult, arErr := enqueueAnalysisAndWaitForCompletion(fileLogger, diEndpoint, diKey, pathOfFileToImport)
	if arErr != nil {
		pdd := processingDoneData{pathOfFileToImport: pathOfFileToImport}
		return []*processingDoneData{&pdd}
	}

	pdds := make([]*processingDoneData, 0, len(analysisResult.Documents))
	for i, documentFromAnalysis := range analysisResult.Documents {
		pdd := processingDoneData{pathOfFileToImport: pathOfFileToImport, doc: &documentFromAnalysis}
		fileLogger.Debugf("%s analyzed, importing document nr %d...", pathOfFileToImportBaseName, i+1)

		if beleg, importErr := importIntoBelegManager(fileLogger, db, dbSemaphore, belegManagerDirectory, pathOfFileToImport, documentFromAnalysis); importErr == nil {
			pdd.beleg = beleg
			fileLogger.Debugf("Document nr %d from %s imported", i+1, pathOfFileToImportBaseName)
		} else {
			fileLogger.WithError(importErr).Warn("Failed to import file")
		}

		pdds = append(pdds, &pdd)
	}

	return pdds
}

func importIntoBelegManager(logger *log.Entry, db *sqlx.DB, dbSemaphore chan struct{}, belegManagerDirectory *os.File, pathOfFileToImport string, analysedDocument diDocument) (*bmDocBeleg, error) {
	if documentIsNoInvoiceErr := diDocumentIsTypeInvoice(logger, analysedDocument); documentIsNoInvoiceErr != nil {
		return nil, documentIsNoInvoiceErr
	}

	// Block if another transaction is in progress, release 'dbSemaphore' via defer
	dbSemaphore <- struct{}{}
	defer func() {
		<-dbSemaphore
	}()

	tx, beginTxErr := beginTransaction(db)
	if beginTxErr != nil {
		return nil, beginTxErr
	}
	defer finishTransaction(tx)

	beleg, err := createOrUpdateBeleg(logger, tx, belegManagerDirectory, pathOfFileToImport, analysedDocument)
	if err != nil {
		return nil, err
	}

	if linkCustomerCategoryErr := linkCategoryToBeleg(logger, tx, analysedDocument, "CustomerName", beleg); linkCustomerCategoryErr != nil {
		return nil, linkCustomerCategoryErr
	}
	if linkVendorCategoryErr := linkCategoryToBeleg(logger, tx, analysedDocument, "VendorName", beleg); linkVendorCategoryErr != nil {
		return nil, linkVendorCategoryErr
	}

	return beleg, nil
}

func createOrUpdateBeleg(logger *log.Entry, tx *sqlx.Tx, belegManagerDirectory *os.File, pathOfFileToImport string, analysedDocument diDocument) (*bmDocBeleg, error) {
	fileToImportStatInfo, fileStatErr := os.Stat(pathOfFileToImport)
	if fileStatErr != nil && !os.IsNotExist(fileStatErr) {
		logger.WithError(fileStatErr).Warnf("Error checking for file %s ", pathOfFileToImport)
		return nil, fileStatErr
	}

	bmDocAssets, fileInfoForAsset, findAssetErr := findBmDocAssets(logger, tx, belegManagerDirectory, pathOfFileToImport)
	if findAssetErr != nil {
		return nil, findAssetErr
	}

	if len(bmDocAssets) == 1 && !bmDocAssets[0].isDeleted() && fileInfoForAsset != nil && fileToImportStatInfo.Size() == fileInfoForAsset.Size() {
		return updateBmDocBeleg(logger, tx, analysedDocument, bmDocAssets[0])
	}

	return createBmDocBelegWithLinkedAsset(logger, tx, belegManagerDirectory, pathOfFileToImport, analysedDocument)
}

func linkCategoryToBeleg(logger *log.Entry, tx *sqlx.Tx, analysedDocument diDocument, fieldName string, beleg *bmDocBeleg) error {
	cat, catErr := findOrCreateBmDocCategory(logger, tx, analysedDocument, fieldName)
	if catErr != nil {
		return catErr
	}
	if createLinkErr := createIgnoreBmDocLink(logger, tx, cat.UUID, beleg.UUID); createLinkErr != nil {
		return createLinkErr
	}

	return nil
}
