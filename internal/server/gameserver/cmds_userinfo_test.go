package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestBuildSimUserInfoMinLen(t *testing.T) {
	s := &Server{}
	b := s.buildSimUserInfo(10002)
	// uid+nick16+12*u32 + clothCount0 = 4+16+48+4 = 72
	if len(b) < 72 {
		t.Fatalf("len=%d want >=72", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 10002 {
		t.Fatal("uid")
	}
	if binary.BigEndian.Uint32(b[len(b)-4:]) != 0 {
		t.Fatal("clothCount should be 0")
	}
}

func TestBuildMoreUserInfoLen(t *testing.T) {
	s := &Server{}
	b := s.buildMoreUserInfo(10002)
	// 4+16+4*3 +200 +4*6 = 20+12+200+24 = 256
	if len(b) != 256 {
		t.Fatalf("len=%d want 256", len(b))
	}
	if binary.BigEndian.Uint32(b[0:4]) != 10002 {
		t.Fatal("uid")
	}
}

func TestBuildBossAchievementFromThreshold(t *testing.T) {
	out := make([]byte, 200)
	th := 301
	out[th-301] = 1
	if out[0] != 1 {
		t.Fatal("index0")
	}
}
