package gameserver

import (
	"testing"

	"niaohao/server/internal/store"
)

func TestLoginBodyPetOffset(t *testing.T) {
	// Simulate fixed header of buildLoginResponse up to petNum
	// Count from code... easier: build two pets and check
	p1 := &store.Pet{CatchTime: 1768449796, PetID: 4, Name: "伊优", Level: 5, DV: 20, Skills: []int{10004, 20002}}
	p2 := &store.Pet{CatchTime: 1768449799, PetID: 7, Name: "小火猴", Level: 5, DV: 20, Skills: []int{10006, 20004}}
	b1 := buildPetInfo(p1)
	b2 := buildPetInfo(p2)
	t.Logf("petInfo len=%d+%d=%d", len(b1), len(b2), len(b1)+len(b2))
	// From buildLoginResponse: before pets is taskList 1000 + fixed fields
	// userID..reserved roughly: 
	// After taskList(1000): petNum(4) + pets + clothes(4) + title(4) + 200 bossAch
	fixedAfterPets := 4 + 4 + 200 // clothes count + title + bossAch stub
	t.Logf("pets+tail=%d", len(b1)+len(b2)+4+fixedAfterPets)
}
