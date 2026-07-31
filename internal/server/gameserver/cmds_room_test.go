package gameserver

import (
	"encoding/binary"
	"testing"
)

func TestBuildRoomAddressLen(t *testing.T) {
	s := &Server{cfg: Config{Port: 1863, AdvertiseHost: "127.0.0.1"}, roomByUID: map[int64]*Client{}}
	ip := s.advertiseIPUint32()
	if ip != binary.BigEndian.Uint32([]byte{127, 0, 0, 1}) {
		t.Fatalf("ip=%d", ip)
	}
	out := make([]byte, roomSessionLen+4+2)
	binary.BigEndian.PutUint32(out[roomSessionLen:roomSessionLen+4], ip)
	binary.BigEndian.PutUint16(out[roomSessionLen+4:roomSessionLen+6], 1863)
	if len(out) != 30 {
		t.Fatalf("len=%d want 30", len(out))
	}
}

func TestSessionMatchPad24(t *testing.T) {
	s := &Server{}
	if !s.sessionMatch(1, make([]byte, 24)) {
		t.Fatal("nil store should pass")
	}
}

func TestResolveFitmentOwner(t *testing.T) {
	s := &Server{}
	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, 10002)
	if s.resolveFitmentOwner(7, body) != 10002 {
		t.Fatal("uid")
	}
	binary.BigEndian.PutUint32(body, 500001)
	if s.resolveFitmentOwner(7, body) != 7 {
		t.Fatal("style id → self")
	}
}
