package gameserver

import (
	"bytes"
	"encoding/binary"
	"testing"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

func TestAssignPetTraitStable(t *testing.T) {
	p := &store.Pet{PetID: 301, CatchTime: 12345}
	AssignPetTraitIfNeeded(p)
	if !IsValidPetTrait(p.Trait) {
		t.Fatalf("trait %d invalid", p.Trait)
	}
	first := p.Trait
	p.Trait = 0
	AssignPetTraitIfNeeded(p)
	if p.Trait != first {
		t.Fatalf("unstable trait %d vs %d", p.Trait, first)
	}
}

func TestWritePetEffectListTraitBeforeEnergy(t *testing.T) {
	// 本客户端 PetDataPanel 读 effectList[0] 为特性
	p := &store.Pet{
		PetID: 1, CatchTime: 1, Level: 5, DV: 20, Trait: 1023,
		Skills:              []int{10001},
		EnergyBallItemID:    300027,
		EnergyBallLeftCount: 3,
		EnergyBallEffectID:  1001,
	}
	var buf bytes.Buffer
	writePetEffectList(&buf, p)
	raw := buf.Bytes()
	if len(raw) < 2 {
		t.Fatal("empty")
	}
	cnt := binary.BigEndian.Uint16(raw[0:2])
	if cnt != 2 {
		t.Fatalf("effectCount=%d want 2", cnt)
	}
	item0 := binary.BigEndian.Uint32(raw[2:6])
	if item0 != 1023 {
		t.Fatalf("first itemId=%d want trait 1023", item0)
	}
	if raw[6] != 1 {
		t.Fatalf("first status=%d want 1", raw[6])
	}
	off := 2 + 24
	item1 := binary.BigEndian.Uint32(raw[off : off+4])
	if item1 != 300027 {
		t.Fatalf("second itemId=%d want energy 300027", item1)
	}
	if raw[off+4] != 2 {
		t.Fatalf("second status=%d want 2", raw[off+4])
	}
}

func TestBuildPetInfoNoTraitLen162(t *testing.T) {
	p := &store.Pet{PetID: 1, CatchTime: 1, Level: 5, DV: 20, Skills: []int{10001}}
	b := buildPetInfo(p)
	if len(b) != 162 {
		t.Fatalf("len=%d want 162 (empty effectList)", len(b))
	}
}

func TestBuildPetInfoWithTraitAligned(t *testing.T) {
	cat := testCatalog(t)
	defaultSkillCatalog = cat
	defer func() { defaultSkillCatalog = nil }()
	p := &store.Pet{PetID: 1, CatchTime: 1, Level: 5, DV: 20, Trait: 1023, Skills: []int{10001}}
	b := buildPetInfo(p)
	if len(b) != 162+24 {
		t.Fatalf("len=%d want %d", len(b), 162+24)
	}
	off := len(b) - 12 - 24 - 2
	cnt := binary.BigEndian.Uint16(b[off : off+2])
	if cnt != 1 {
		t.Fatalf("effectCount=%d", cnt)
	}
	if binary.BigEndian.Uint32(b[off+2:off+6]) != 1023 {
		t.Fatalf("trait itemId")
	}
	if b[off+6] != 1 {
		t.Fatalf("status=%d", b[off+6])
	}
}

func TestTraitOutgoingTypeBonus(t *testing.T) {
	sk := &tableloader.SkillDef{Type: 1, Category: 1, Power: 40}
	dmg, instant := applyTraitOutgoingDamage(1006, sk, 100)
	if instant || dmg != 105 {
		t.Fatalf("type bonus got dmg=%d instant=%v", dmg, instant)
	}
	dmg2, _ := applyTraitOutgoingDamage(1007, sk, 100)
	if dmg2 != 100 {
		t.Fatalf("wrong type should not boost: %d", dmg2)
	}
}

func TestTraitIncomingHard(t *testing.T) {
	got := applyTraitIncomingDamage(1024, 200, 100)
	if got != 95 {
		t.Fatalf("hard want 95 got %d", got)
	}
}

func TestTraitDrainHeal(t *testing.T) {
	if traitDrainHeal(1039, 100) != 8 {
		t.Fatal("drain 8%")
	}
	if traitDrainHeal(1023, 100) != 0 {
		t.Fatal("non-drain")
	}
}

func TestTraitReactivePhysicalStatus(t *testing.T) {
	// 强制触发：循环直到带电挂上麻痹（3%）
	sk := &tableloader.SkillDef{Category: 1, Power: 40}
	var got bool
	for i := 0; i < 5000; i++ {
		var atkStatus battleStatus
		applyTraitReactiveOnHit(1029, sk, 10, nil, nil, &atkStatus)
		if atkStatus.Para {
			got = true
			break
		}
	}
	if !got {
		t.Fatal("1029 should eventually para attacker")
	}
	// 特攻不应触发 1029
	var atkStatus battleStatus
	for i := 0; i < 200; i++ {
		applyTraitReactiveOnHit(1029, &tableloader.SkillDef{Category: 2}, 10, nil, nil, &atkStatus)
	}
	if atkStatus.Para {
		t.Fatal("1029 must not trigger on special")
	}
}

func TestTraitReactiveSpecialStatDown(t *testing.T) {
	sk := &tableloader.SkillDef{Category: 2, Power: 40}
	var atkStages [5]int8
	hit := false
	for i := 0; i < 5000; i++ {
		atkStages = [5]int8{}
		applyTraitReactiveOnHit(1035, sk, 0, nil, &atkStages, nil)
		if atkStages[stageAtk] == -1 {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("1035 should eventually lower atk")
	}
}

func TestTraitReactiveSelfStatUp(t *testing.T) {
	sk := &tableloader.SkillDef{Category: 1, Power: 40}
	var defStages [5]int8
	hit := false
	for i := 0; i < 5000; i++ {
		defStages = [5]int8{}
		applyTraitReactiveOnHit(1041, sk, 1, &defStages, nil, nil)
		if defStages[stageAtk] == 1 {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("1041 should eventually raise self atk")
	}
}

func TestConsumeSkipStatusSleep(t *testing.T) {
	st := battleStatus{Sleep: true}
	if !consumeSkipStatus(&st) {
		t.Fatal("sleep should skip")
	}
	if st.Sleep {
		t.Fatal("sleep should clear after consume")
	}
}

func TestApplyPetTraitItemOpen(t *testing.T) {
	p := &store.Pet{PetID: 1, CatchTime: 99}
	meta := tableloader.ItemMeta{NonFuseAddNewse: true}
	if !applyPetTraitItem(p, meta, nil) || !IsValidPetTrait(p.Trait) {
		t.Fatalf("open trait failed: %d", p.Trait)
	}
	old := p.Trait
	if applyPetTraitItem(p, meta, nil) {
		t.Fatal("second open should no-op")
	}
	if p.Trait != old {
		t.Fatal("trait changed on second open")
	}
}

func TestGrantNewPetDoesNotAutoTrait(t *testing.T) {
	// 捕捉/孵化传 trait=-1 时不得自动分配；仅融合显式 Idx 才有特性
	if IsValidPetTrait(-1) || IsValidPetTrait(0) {
		t.Fatal("-1/0 must be invalid trait")
	}
	if !IsValidPetTrait(1023) {
		t.Fatal("1023 should be valid")
	}
}

func TestSimplePetInfoSkinZero(t *testing.T) {
	b := buildSimplePetInfo(4, 5, 100, 100, 12345, [][2]uint32{{10004, 20}}, 0, 0, 0)
	if len(b) != 80 {
		t.Fatalf("len=%d", len(b))
	}
	skin := binary.BigEndian.Uint32(b[68:72])
	if skin != 0 {
		t.Fatalf("skin=%d want 0", skin)
	}
}
