package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenLogger(t *testing.T) {
	// Test: empty spec disables logging (no logger, no file to close)
	l, c := openLogger("")
	assert.Nil(t, l)
	assert.Nil(t, c)

	// Test: a file path creates an appendable log file
	p := filepath.Join(t.TempDir(), "gateway.log")
	l, c = openLogger(p)
	assert.NotNil(t, l)
	assert.NotNil(t, c)
	// Close before the test returns so t.TempDir cleanup can delete the file;
	// on Windows RemoveAll fails while the handle is still open.
	defer c.Close()
	l.Print("hello")
	b, err := os.ReadFile(p)
	assert.NoError(t, err)
	assert.Contains(t, string(b), "hello")

	// Test: an unwritable path degrades to nil, never a crash
	l, c = openLogger(filepath.Join(t.TempDir(), "nope", "deep", "x.log"))
	assert.Nil(t, l)
	assert.Nil(t, c)
}
