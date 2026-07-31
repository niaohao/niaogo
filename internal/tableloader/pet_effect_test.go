package tableloader

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testXMLDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "tables", "xml")
}

func TestEnergyBallForItem(t *testing.T) {
	c := New(testXMLDir(t))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	idx, times, ok := c.EnergyBallForItem(300030)
	if !ok || idx != 1001 || times != 20 {
		t.Fatalf("300030 -> idx=%d times=%d ok=%v", idx, times, ok)
	}
	idx, times, ok = c.EnergyBallForItem(300055)
	if !ok || idx != 1052 || times != 20 {
		t.Fatalf("300055 -> idx=%d times=%d ok=%v", idx, times, ok)
	}
	if _, _, ok := c.EnergyBallForItem(300001); ok {
		t.Fatal("potion should not be energy ball")
	}
}
