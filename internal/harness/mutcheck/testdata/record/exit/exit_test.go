package exit

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	m.Run()
	os.Exit(3)
}

func TestPasses(t *testing.T) {}
