package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
)

func TestLoginResponsePetCatchTimes(t *testing.T) {
	s := &Server{}
	// Inject pets by temporarily using buildLogin pieces: call buildPetInfo after task offset
	pets := []store.Pet{
		{CatchTime: 1768449796, PetID: 4, Name: "伊优", Level: 5, DV: 20, Skills: []int{10004}},
		{CatchTime: 1784961846, PetID: 50, Name: "p50", Level: 5, DV: 20, Skills: []int{10200, 10201}},
		{CatchTime: 1784868204, PetID: 70, Name: "p70", Level: 100, DV: 20, Skills: []int{10006, 10167, 10166, 10171}},
		{CatchTime: 1784902788, PetID: 261, Name: "p261", Level: 100, DV: 20, Skills: []int{10127, 10710, 10716, 10711}},
	}
	u := &store.User{UserID: 10002, Nickname: "尼奥号", RegisterTime: 1784815487, Energy: 100, MapID: 1, PosX: 480, PosY: 280}
	body := s.buildLoginResponse(u)
	// Without store, no pets — build manually appended version
	taskStart := 438
	petNumOff := taskStart + 1000
	// Rebuild with pets: take header through taskList, append pets
	hdr := body[:petNumOff]
	var out []byte
	out = append(out, hdr...)
	b4 := make([]byte, 4)
	binary.BigEndian.PutUint32(b4, uint32(len(pets)))
	out = append(out, b4...)
	for i := range pets {
		out = append(out, buildPetInfo(&pets[i])...)
	}
	binary.BigEndian.PutUint32(b4, 0) // clothes
	out = append(out, b4...)
	binary.BigEndian.PutUint32(b4, 0) // title
	out = append(out, b4...)
	out = append(out, make([]byte, 200)...)

	off := petNumOff
	n := binary.BigEndian.Uint32(out[off : off+4])
	off += 4
	t.Logf("petNum=%d bodyLen=%d", n, len(out))
	if n != 4 {
		t.Fatalf("petNum=%d", n)
	}
	for i := 0; i < int(n); i++ {
		p := out[off : off+162]
		pid := binary.BigEndian.Uint32(p[0:4])
		skillNum := binary.BigEndian.Uint32(p[96:100])
		ct := binary.BigEndian.Uint32(p[132:136])
		t.Logf("pet[%d] id=%d skillNum=%d catch=%d", i, pid, skillNum, ct)
		if ct != uint32(pets[i].CatchTime) {
			t.Fatalf("catch mismatch")
		}
		off += 162
	}
}
