package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestBuildChatInfo(t *testing.T) {
	b := buildChatInfo(10001, "尼奥", 0, []byte("hi"))
	if binary.BigEndian.Uint32(b[0:4]) != 10001 {
		t.Fatal("sender")
	}
	if binary.BigEndian.Uint32(b[20:24]) != 0 {
		t.Fatal("to")
	}
	if binary.BigEndian.Uint32(b[24:28]) != 2 {
		t.Fatal("len")
	}
	if string(b[28:]) != "hi" {
		t.Fatal("msg")
	}
}

func TestBuildChangeClothInfo(t *testing.T) {
	b := buildChangeClothInfo(7, []uint32{100001, 100002})
	if len(b) != 8+16 {
		t.Fatalf("len=%d", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 7 {
		t.Fatal("uid")
	}
	if binary.BigEndian.Uint32(b[4:8]) != 2 {
		t.Fatal("n")
	}
}
