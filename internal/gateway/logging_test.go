package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenLogger(t *testing.T) {
	// Test: empty spec disables logging
	assert.Nil(t, openLogger(""))

	// Test: a file path creates an appendable log file
	p := filepath.Join(t.TempDir(), "gateway.log")
	l := openLogger(p)
	assert.NotNil(t, l)
	l.Print("hello")
	b, err := os.ReadFile(p)
	assert.NoError(t, err)
	assert.Contains(t, string(b), "hello")

	// Test: an unwritable path degrades to nil, never a crash
	assert.Nil(t, openLogger(filepath.Join(t.TempDir(), "nope", "deep", "x.log")))
}
