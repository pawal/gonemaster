package nstest

import (
	"fmt"
	"testing"
)

// errFatal aborts a fakeTB the way testing.T.Fatalf aborts a real test.
var errFatal = fmt.Errorf("fatal")

// fakeTB panics instead of failing the test, so the helpers' failure paths can
// be asserted on.
type fakeTB struct {
	testing.TB
	msg string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
	panic(errFatal)
}
