package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestBuildTopWarRankBodyEmpty(t *testing.T) {
	out := buildTopWarRankBody(0, nil)
	if len(out) != 8 {
		t.Fatalf("empty body len=%d want 8", len(out))
	}
	if binary.BigEndian.Uint32(out[0:4]) != 0 {
		t.Fatal("selfRank")
	}
	if binary.BigEndian.Uint32(out[4:8]) != 0 {
		t.Fatal("count")
	}
}

func TestBuildTopWarRankBodyEntries(t *testing.T) {
	list := []store.TopWarRankEntry{
		{UserID: 1001, Nickname: "Alice", Score: 1500},
		{UserID: 1002, Nickname: "", Score: 900},
	}
	out := buildTopWarRankBody(2, list)
	if binary.BigEndian.Uint32(out[0:4]) != 2 {
		t.Fatal("selfRank")
	}
	if binary.BigEndian.Uint32(out[4:8]) != 2 {
		t.Fatal("count")
	}
	off := 8
	// Alice: u16 len=5 + "Alice" + score
	nickLen := int(binary.BigEndian.Uint16(out[off : off+2]))
	off += 2
	if nickLen != 5 || string(out[off:off+nickLen]) != "Alice" {
		t.Fatalf("nick1 %q len=%d", out[off:off+nickLen], nickLen)
	}
	off += nickLen
	if binary.BigEndian.Uint32(out[off:off+4]) != 1500 {
		t.Fatal("score1")
	}
	off += 4
	// empty nick → uid string "1002"
	nickLen = int(binary.BigEndian.Uint16(out[off : off+2]))
	off += 2
	if string(out[off:off+nickLen]) != "1002" {
		t.Fatalf("nick2 fallback got %q", out[off:off+nickLen])
	}
	off += nickLen
	if binary.BigEndian.Uint32(out[off:off+4]) != 900 {
		t.Fatal("score2")
	}
	if off+4 != len(out) {
		t.Fatalf("trailing bytes: off=%d len=%d", off, len(out))
	}
}

func TestClampTopLevelViaStore(t *testing.T) {
	if store.ClampTopLevel(-1) != 0 {
		t.Fatal("min")
	}
	if store.ClampTopLevel(1_000_000) != store.TopLevelMax {
		t.Fatal("max")
	}
}
