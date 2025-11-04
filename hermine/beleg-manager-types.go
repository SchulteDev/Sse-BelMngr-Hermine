package hermine

const bmDocRFC3339Milli = "2006-01-02T15:04:05.000Z"

type sqlxSelecter interface {
	Select(dest any, query string, args ...any) error
}

type sqlxGetter interface {
	Get(dest any, query string, args ...any) error
}

type bmDocEntity struct {
	ID                uint32  `db:"id"`                // INTEGER, primary key
	UUID              string  `db:"uuid"`              // TEXT
	Name              string  `db:"name"`              // TEXT
	DocType           *uint8  `db:"docType"`           // INTEGER, nullable
	DeleteState       *uint8  `db:"deleteState"`       // INTEGER
	DocDate           *string `db:"docDate"`           // TEXT, nullable
	TimestampCreated  *string `db:"timestampCreated"`  // TEXT, nullable
	Unread            *uint8  `db:"unread"`            // INTEGER, nullable
	Sync              *uint8  `db:"sync"`              // INTEGER, nullable
	NeedUpSync        *uint8  `db:"needUpSync"`        // INTEGER, nullable
	NeedDownSync      *uint8  `db:"needDownSync"`      // INTEGER, nullable
	TimestampLastSync *string `db:"timestampLastSync"` // TEXT, nullable
}

func (e bmDocEntity) isDeleted() bool {
	return e.DeleteState != nil && *e.DeleteState != 0
}

// bmDocAsset corresponds to the BmDoc_Asset table.
type bmDocAsset struct {
	bmDocEntity
	TargetDocType *uint8  `db:"targetDocType"` // INTEGER, nullable
	OcrState      *uint8  `db:"ocrState"`      // INTEGER, nullable
	InternalPath  *string `db:"internalPath"`  // TEXT
	FileSyncState *uint8  `db:"fileSyncState"` // INTEGER, default (non-null)
}

// bmDocBeleg corresponds to the BmDoc_Beleg table.
type bmDocBeleg struct {
	bmDocEntity
	Number    *string  `db:"number"`    // TEXT, nullable
	Amount    *float64 `db:"amount"`    // DOUBLE, nullable
	Netto     *uint8   `db:"netto"`     // INTEGER, nullable
	VAT       *float64 `db:"vat"`       // DOUBLE, nullable
	Comment   *string  `db:"comment"`   // TEXT, nullable
	BelegDate *string  `db:"belegDate"` // TEXT, nullable
}

// bmDocLink corresponds to the BmDoc_LinkTable table.
type bmDocLink struct {
	ID         uint32 `db:"id"`         // INTEGER, primary key
	SourceUUID string `db:"sourceUuid"` // TEXT
	TargetUUID string `db:"targetUuid"` // TEXT
}

// bmDocCategory corresponds to the BmDoc_Kategorie table.
type bmDocCategory struct {
	bmDocEntity
}
