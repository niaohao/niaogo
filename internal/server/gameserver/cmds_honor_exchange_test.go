package gameserver

import (
	"encoding/binary"
	"path/filepath"
	"testing"
)

func TestHonorExchangeRemain(t *testing.T) {
	if honorExchangeRemain(1, 0) != 1 {
		t.Fatal("fresh Max=1")
	}
	if honorExchangeRemain(1, 1) != 0 {
		t.Fatal("used up")
	}
	if honorExchangeRemain(999, 50) != 999 {
		t.Fatal("unlimited stays 999")
	}
}

func TestBuildHonorExchangeResponse(t *testing.T) {
	item := buildHonorExchangeResponse(42, 0, 0, 300001)
	if binary.BigEndian.Uint32(item[0:4]) != 42 {
		t.Fatal("honour")
	}
	if binary.BigEndian.Uint32(item[4:8]) != 0 {
		t.Fatal("monID should be 0 for item")
	}
	if binary.BigEndian.Uint32(item[12:16]) != 1 {
		t.Fatal("itemCount")
	}
	if binary.BigEndian.Uint32(item[16:20]) != 300001 {
		t.Fatal("itemID")
	}
	if len(item) != 16+8+8 {
		t.Fatalf("item packet size want 32 got %d (need mintmark+pad)", len(item))
	}

	pet := buildHonorExchangeResponse(90, 196, 1700000001, 0)
	if binary.BigEndian.Uint32(pet[4:8]) != 196 {
		t.Fatal("pet monID")
	}
	if binary.BigEndian.Uint32(pet[8:12]) != 1700000001 {
		t.Fatal("pet capTime")
	}
	if binary.BigEndian.Uint32(pet[12:16]) != 0 {
		t.Fatal("pet itemCount must be 0")
	}
	if len(pet) != 24 {
		t.Fatalf("pet packet size want 24 got %d", len(pet))
	}
}

func TestLoadHonorExchangeFromTables(t *testing.T) {
	xml := filepath.Join("..", "..", "..", "tables", "xml", "TopFightExchangeXMLInfo.xml")
	s := &Server{cfg: Config{DataDir: filepath.Join("..", "..", "..", "data")}}
	// reset package cache for isolated load
	honorExMu.Lock()
	honorExLoaded = false
	honorExByID = nil
	honorExOrder = nil
	honorExMu.Unlock()

	entries := s.listHonorExchangeEntries()
	if len(entries) < 10 {
		t.Fatalf("expected exchange table loaded from %s, got %d (cwd relative)", xml, len(entries))
	}
	e, ok := s.getHonorExchangeEntry(entries[0].ID)
	if !ok || e.ItemID <= 0 || e.MaxExchange <= 0 {
		t.Fatalf("bad entry %+v", e)
	}
}

func TestAimatResponseLayout(t *testing.T) {
	out := make([]byte, 20)
	binary.BigEndian.PutUint32(out[0:4], 1001)
	binary.BigEndian.PutUint32(out[4:8], 100245)
	binary.BigEndian.PutUint32(out[8:12], 1)
	binary.BigEndian.PutUint32(out[12:16], 320)
	binary.BigEndian.PutUint32(out[16:20], 240)
	if binary.BigEndian.Uint32(out[0:4]) != 1001 || binary.BigEndian.Uint32(out[4:8]) != 100245 {
		t.Fatal("layout")
	}
}
