package store

import "testing"

func TestRemapLowUID(t *testing.T) {
	if remapLowUID(10002) != 50002 {
		t.Fatal(remapLowUID(10002))
	}
	if remapLowUID(50002) != 50002 {
		t.Fatal("already high")
	}
	if MinUserID != 50000 {
		t.Fatal(MinUserID)
	}
}
