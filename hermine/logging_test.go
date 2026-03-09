package hermine

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

func Test_newDebuggingNullLogger(t *testing.T) {
	logger, hook := newDebuggingNullLogger(t)

	assert.NotNil(t, logger)
	assert.NotNil(t, hook)
}

func newDebuggingNullLogger(t *testing.T) (*log.Logger, *test.Hook) {
	t.Helper()
	return newNullLogger(t, log.DebugLevel)
}

func newNullLogger(t *testing.T, level log.Level) (*log.Logger, *test.Hook) {
	t.Helper()

	nullLogger, nullLoggerHook := test.NewNullLogger()
	nullLogger.SetLevel(level)
	testLogger := nullLogger.WithField("test", t.Name())
	return testLogger.Logger, nullLoggerHook
}
