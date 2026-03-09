package hermine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_enqueueAnalysisAndWaitForCompletion(t *testing.T) {
	diEndpoint := os.Getenv("DI_ENDPOINT")
	diKey := os.Getenv("DI_KEY")
	if diEndpoint == "" || diKey == "" {
		t.Skipf("Skipping test because DI_ENDPOINT and/or DI_KEY is not set. DI_ENDPOINT: %s, DI_KEY: %s", diEndpoint, diKey)
	}

	testLogger, _ := newDebuggingNullLogger(t)
	testLoggerEntry := testLogger.WithField("test", t.Name())

	invoiceFilePath := filepath.Join("testdata", invoiceExampleFileName)
	diAr, analysisErr := enqueueAnalysisAndWaitForCompletion(testLoggerEntry, diEndpoint, diKey, invoiceFilePath)

	require.NoError(t, analysisErr)
	require.NotNil(t, diAr)
	assert.Len(t, diAr.Documents, 1)
	assert.NotEmpty(t, diAr.Content)
}
