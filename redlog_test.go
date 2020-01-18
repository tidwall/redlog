package redlog

import (
	"bytes"
	"testing"
)

func TestLog(t *testing.T) {
	buf := &bytes.Buffer{}
	l := New(buf, nil)
	l.Printf("hello world\n")
}
