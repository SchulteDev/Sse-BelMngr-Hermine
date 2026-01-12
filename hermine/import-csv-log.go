package hermine

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
)

type processingDoneData struct {
	pathOfFileToImport string
	beleg              *bmDocBeleg
	doc                *diDocument
}

func (pdd processingDoneData) toCsvLogRow() []string {
	belegAsCsvLog := belegToCsvLog(pdd.beleg)
	docAsCsvLog := diDocumentToCsvLog(pdd.doc)

	logRow := make([]string, 0, 1+len(belegAsCsvLog)+len(docAsCsvLog))
	logRow = append(logRow, pdd.pathOfFileToImport)
	logRow = append(logRow, belegAsCsvLog...)
	logRow = append(logRow, docAsCsvLog...)

	return logRow
}

func logToCsv(belegManagerDirectory *os.File, pdds []*processingDoneData) {
	csvLogFileName := fmt.Sprintf("_import-log-%s.csv", time.Now().Format(flatDateTime))
	csvLogFilePath := filepath.Join(belegManagerDirectory.Name(), csvLogFileName)
	csvLogFile, openCsvFileErr := os.Create(csvLogFilePath)
	if openCsvFileErr != nil {
		log.WithError(openCsvFileErr).Warnf("Failed to open CSV log file %s", csvLogFilePath)
		return
	}
	defer func() {
		if err := csvLogFile.Close(); err != nil {
			log.WithError(err).Debugf("Failed to close CSV log file %s", csvLogFilePath)
		}
	}()

	writeToCsvLog(csvLogFile, pdds)
}

func writeToCsvLog(csvLogFile *os.File, pdds []*processingDoneData) {
	csvLogFileWriter := csv.NewWriter(csvLogFile)
	defer csvLogFileWriter.Flush()

	csvHeaders := []string{"OriginalPath", "BelegID", "BelegName", "BelegDate", "InvoiceTotal", "InvoiceTotalConfidence", "VatRate"}
	if writeHeadersErr := csvLogFileWriter.Write(csvHeaders); writeHeadersErr != nil {
		log.WithError(writeHeadersErr).Warn("Failed to write CSV headers")
	}

	for _, pdd := range pdds {
		row := pdd.toCsvLogRow()
		if writeRowErr := csvLogFileWriter.Write(row); writeRowErr != nil {
			log.WithError(writeRowErr).Warnf("Failed to write row to CSV file %s", csvLogFile.Name())
		}
	}

	log.Infof("Wrote CSV log file %s", csvLogFile.Name())
}

func belegToCsvLog(beleg *bmDocBeleg) []string {
	if beleg == nil {
		return []string{"", "", "", ""}
	}

	return []string{
		strconv.FormatUint(uint64(beleg.ID), 10),
		beleg.Name,
		*beleg.BelegDate,
		convertFloatPointerToString(beleg.Amount),
	}
}

func diDocumentToCsvLog(d *diDocument) []string {
	if d == nil {
		return []string{"", "", ""}
	}

	return []string{
		convertFloatPointerToString(d.getGrossConfidence()),
		convertFloatPointerToString(d.getVat()),
	}
}

func convertFloatPointerToString(value *float64) string {
	if value != nil {
		return fmt.Sprintf("%.2f", *value)
	}

	return ""
}

func diDocumentIsTypeInvoice(logger *log.Entry, d diDocument) error {
	if !d.isTypeInvoice() {
		err := fmt.Errorf("not an invoice, but %s", d.DocType)
		logger.WithError(err).Debug()
		return err
	}

	return nil
}
