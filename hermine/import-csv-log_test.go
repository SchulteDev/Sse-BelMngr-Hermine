package hermine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBelegToCsvLog_NilBelegDate(t *testing.T) {
	beleg := &bmDocBeleg{
		bmDocEntity: bmDocEntity{
			ID:   123,
			Name: "Test Beleg",
		},
		BelegDate: nil, // This should trigger the panic if not handled
		Amount:    new(12.34),
	}

	assert.NotPanics(t, func() {
		row := belegToCsvLog(beleg)
		assert.Equal(t, []string{"123", "Test Beleg", "", "12.34"}, row)
	})
}

func TestToCsvLogRow_Consistency(t *testing.T) {
	pdd := &processingDoneData{
		pathOfFileToImport: "C:/test.pdf",
		beleg: &bmDocBeleg{
			bmDocEntity: bmDocEntity{
				ID:   123,
				Name: "Test Beleg",
			},
			BelegDate: new("2025-01-01"),
			Amount:    new(12.34),
		},
		doc: nil,
	}

	row := pdd.toCsvLogRow()
	require.Len(t, row, 7)
	assert.Equal(t, "C:/test.pdf", row[0])
	assert.Equal(t, "123", row[1])
	assert.Equal(t, "Test Beleg", row[2])
	assert.Equal(t, "2025-01-01", row[3])
	assert.Equal(t, "12.34", row[4])
	assert.Empty(t, row[5])
	assert.Empty(t, row[6])
}
