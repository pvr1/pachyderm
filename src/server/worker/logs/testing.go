package logs

import (
	"fmt"
	"io"
	"time"

	"github.com/pachyderm/pachyderm/v2/src/internal/errors"
	"github.com/pachyderm/pachyderm/v2/src/server/worker/common"
)

// MockLogger is an implementation of the TaggedLogger interface for use in
// tests.  Loggers are often passed to callbacks, so you can check that the
// logger has been configured with the right tags in these cases.  In addition,
// you can set the Writer field so that log statements go directly to stdout
// (or some other location) for debugging purposes.
type MockLogger struct {
	// These fields are exposed so that tests can set them or make assertions
	Writer      io.Writer
	PipelineJob string
	Data        []*common.Input
	UserCode    bool
}

// Not used - forces a compile-time error in this file if MockLogger does not
// implement TaggedLogger
var _ TaggedLogger = &MockLogger{}

// NewMockLogger constructs a MockLogger object for use by tests.
func NewMockLogger() *MockLogger {
	return &MockLogger{}
}

// Write fulfills the io.Writer interface for TaggedLogger, and will optionally
// write to the configured ml.Writer, otherwise it pretends that it succeeded.
func (ml *MockLogger) Write(p []byte) (_ int, retErr error) {
	if ml.Writer != nil {
		return ml.Writer.Write(p)
	}
	return len(p), nil
}

// Logf optionally logs a statement using string formatting
func (ml *MockLogger) Logf(formatString string, args ...interface{}) {
	if ml.Writer != nil {
		params := []interface{}{time.Now().Format(time.StampMilli), ml.PipelineJob, ml.Data, ml.UserCode}
		params = append(params, args...)
		str := fmt.Sprintf("LOGF %s (%v, %v, %v): "+formatString+"\n", params...)
		ml.Writer.Write([]byte(str))
	}
}

// Errf optionally logs an error statement using string formatting
func (ml *MockLogger) Errf(formatString string, args ...interface{}) {
	if ml.Writer != nil {
		params := []interface{}{time.Now().Format(time.StampMilli), ml.PipelineJob, ml.Data, ml.UserCode}
		params = append(params, args...)
		str := fmt.Sprintf("ERRF %s (%v, %v, %v): "+formatString+"\n", params...)
		ml.Writer.Write([]byte(str))
	}
}

// LogStep will log before and after the given callback function runs, using
// the name provided
func (ml *MockLogger) LogStep(name string, cb func() error) (retErr error) {
	ml.Logf("started %v", name)
	defer func() {
		if retErr != nil {
			retErr = errors.EnsureStack(retErr)
			ml.Logf("errored %v: %v", name, retErr)
		} else {
			ml.Logf("finished %v", name)
		}
	}()
	return cb()
}

// clone is used by the With* member functions to duplicate the current logger.
func (ml *MockLogger) clone() *MockLogger {
	result := &MockLogger{}
	*result = *ml
	return result
}

// WithPipelineJob duplicates the MockLogger and returns a new one tagged with
// the given pipeline job ID.
func (ml *MockLogger) WithPipelineJob(pipelineJobID string) TaggedLogger {
	result := ml.clone()
	result.PipelineJob = pipelineJobID
	return result
}

// WithData duplicates the MockLogger and returns a new one tagged with the
// given input data.
func (ml *MockLogger) WithData(data []*common.Input) TaggedLogger {
	result := ml.clone()
	result.Data = data
	return result
}

// WithUserCode duplicates the MockLogger and returns a new one tagged to
// indicate that the log statements came from user code.
func (ml *MockLogger) WithUserCode() TaggedLogger {
	result := ml.clone()
	result.UserCode = true
	return result
}

// PipelineJobID returns the currently tagged pipeline job ID for the logger.
// This is redundant for MockLogger, as you can access ml.PipelineJob directly,
// but it is needed for the TaggedLogger interface.
func (ml *MockLogger) PipelineJobID() string {
	return ml.PipelineJob
}
