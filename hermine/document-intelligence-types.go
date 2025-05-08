package hermine

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"strconv"
	"strings"
	"time"
)

const documentTypeInvoice = "invoice"

type diAnalysisStatus struct {
	Status              string           `json:"status"`
	CreatedDateTime     time.Time        `json:"createdDateTime"`
	LastUpdatedDateTime time.Time        `json:"lastUpdatedDateTime"`
	AnalyzeResult       *diAnalyzeResult `json:"analyzeResult"`
}

type diAnalyzeResult struct {
	APIVersion      string           `json:"apiVersion"`
	ModelID         string           `json:"modelId"`
	StringIndexType string           `json:"stringIndexType"`
	Content         string           `json:"content"`
	ContentFormat   string           `json:"contentFormat"`
	Pages           []map[string]any `json:"pages"`
	Tables          []map[string]any `json:"tables"`
	Documents       []diDocument     `json:"documents"`
}

type diDocument struct {
	DocType         string                     `json:"docType"`
	BoundingRegions []diBoundingRegion         `json:"boundingRegions"`
	Fields          map[string]diDocumentField `json:"fields"`
	Confidence      float64                    `json:"confidence"`
	Spans           []diSpan                   `json:"spans"`
}

type diBoundingRegion struct {
	PageNumber int       `json:"pageNumber"`
	Polygon    []float64 `json:"polygon"`
}

type diDocumentField struct {
	Type            string             `json:"type"`
	Content         string             `json:"content"`
	BoundingRegions []diBoundingRegion `json:"boundingRegions"`
	Confidence      float64            `json:"confidence"`
	Spans           []diSpan           `json:"spans"`
	// Specialized field Fields
	ValueString   *string                `json:"valueString,omitempty"`
	ValueDate     *string                `json:"valueDate,omitempty"`
	ValueNumber   *float64               `json:"valueNumber,omitempty"`
	ValueCurrency *diCurrency            `json:"valueCurrency,omitempty"`
	ValueAddress  *diAddress             `json:"valueAddress,omitempty"`
	ValueArray    *[]diDocumentFieldItem `json:"valueArray,omitempty"`
}

type diSpan struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

type diCurrency struct {
	CurrencySymbol string  `json:"currencySymbol"`
	Amount         float64 `json:"amount"`
	CurrencyCode   string  `json:"currencyCode"`
}

type diAddress struct {
	HouseNumber   string `json:"houseNumber"`
	Road          string `json:"road"`
	City          string `json:"city"`
	CountryRegion string `json:"countryRegion"`
	StreetAddress string `json:"streetAddress"`
}

type diDocumentFieldItem struct {
	Type            string                     `json:"type"`
	ValueObject     map[string]diDocumentField `json:"valueObject"`
	Content         string                     `json:"content"`
	BoundingRegions []diBoundingRegion         `json:"boundingRegions"`
	Confidence      float64                    `json:"confidence"`
	Spans           []diSpan                   `json:"spans"`
}

func (d *diAnalysisStatus) isStatusRunning() bool {
	return d.Status == "running"
}

func (d *diAnalysisStatus) isStatusSucceeded() bool {
	return d.Status == "succeeded"
}

func (d *diDocument) createComment() string {
	items := d.Fields["Items"].ValueArray
	names := make([]string, len(*items))
	for n, item := range *items {
		itemDescription := item.ValueObject["Description"].Content
		itemContent := strings.ReplaceAll(itemDescription, "\n", " ")
		names[n] = "- " + itemContent
	}
	itemNamesTextBlock := strings.Join(names, "\n")

	confidenceText := "-"
	if grossConfidence := d.getGrossConfidence(); grossConfidence != nil {
		confidenceText = fmt.Sprintf("%.2f", *grossConfidence)
	}

	return fmt.Sprintf("%s\n\nInvoiceTotal confidence: %s", itemNamesTextBlock, confidenceText)
}

func (d *diDocument) createInvoiceName() string {
	fields := d.Fields

	vendorName := fields["VendorName"].Content
	vendorName = strings.ReplaceAll(vendorName, "\n", " ")

	customerName := fields["CustomerName"].Content
	customerName = strings.ReplaceAll(customerName, "\n", " ")

	items := d.Fields["Items"].ValueArray
	if items != nil && len(*items) == 1 {
		item := (*items)[0]
		itemDescription := item.ValueObject["Description"].Content
		itemContent := strings.ReplaceAll(itemDescription, "\n", " ")
		if len(itemContent) > 40 {
			itemContent = itemContent[:37] + "..."
		}
		return fmt.Sprintf("%s from %sto %s", itemContent, vendorName, customerName)
	}

	return fmt.Sprintf("Invoice %s from %s to %s", fields["InvoiceId"].Content, vendorName, customerName)
}

func (d *diDocument) getContentFieldCommaSeperated(fieldName string) string {
	rawContent := d.Fields[fieldName].Content
	commaContent := strings.ReplaceAll(rawContent, "\n", ", ")
	commaContent = strings.TrimRight(commaContent, ", ")

	return commaContent
}

func (d *diDocument) getGross() *float64 {
	fields := d.Fields
	if field, exists := fields["InvoiceTotal"]; exists {
		return &field.ValueCurrency.Amount
	}

	log.Debug("Field 'InvoiceTotal' not found in document analysis for gross")
	return nil
}

func (d *diDocument) getGrossConfidence() *float64 {
	fields := d.Fields
	if field, exists := fields["InvoiceTotal"]; exists {
		return &field.Confidence
	}

	log.Debug("Field 'InvoiceTotal' not found in document analysis for gross confidence")
	return nil
}

func (d *diDocument) getVat() *float64 {
	taxDetails, taxDetailsExists := d.Fields["TaxDetails"]
	if !taxDetailsExists {
		return nil
	}

	if len(*taxDetails.ValueArray) != 1 {
		log.Debugf("Not exact one but %d TaxDetails from analysis", len(*taxDetails.ValueArray))
		return nil
	}

	taxDetail := (*taxDetails.ValueArray)[0]
	taxRateObject := taxDetail.ValueObject["Rate"]
	// vatAsStringWithPercentSign example: "19%"
	vatAsStringWithPercentSign := taxRateObject.Content
	vatAsStringWithoutPercentSign := strings.TrimSuffix(vatAsStringWithPercentSign, "%")
	vatAsStringWithoutPercentSignNormalized := strings.ReplaceAll(vatAsStringWithoutPercentSign, ",", ".")
	vat, err := strconv.ParseFloat(vatAsStringWithoutPercentSignNormalized, 64)
	if err != nil {
		log.WithError(err).Debugf("%v", vatAsStringWithPercentSign)
		return nil
	}

	return &vat
}

func (d *diDocument) isTypeInvoice() bool {
	return d.DocType == documentTypeInvoice
}
