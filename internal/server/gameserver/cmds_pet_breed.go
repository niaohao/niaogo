package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

const (
	breedEggProduceSec = int64(1)  // 生蛋冷却（秒）
	breedEggHatchSec   = int64(60) // 孵化时长（秒）
)

func normalizeBreedEggs(st *store.BreedState) {
	if st == nil {
		return
	}
	if st.Eggs == nil {
		st.Eggs = []store.BreedEggEntry{}
	}
	if len(st.Eggs) == 0 && st.EggID > 0 && st.EggCatchTime > 0 {
		st.Eggs = append(st.Eggs, store.BreedEggEntry{EggID: st.EggID, EggCatchTime: st.EggCatchTime})
	}
	if st.Intimacy < 1 {
		st.Intimacy = 1
	}
	if st.Intimacy > 5 {
		st.Intimacy = 5
	}
}

func findBreedEgg(eggs []store.BreedEggEntry, catchTime int64) (store.BreedEggEntry, bool) {
	for _, e := range eggs {
		if e.EggCatchTime == catchTime {
			return e, true
		}
	}
	return store.BreedEggEntry{}, false
}

func removeBreedEgg(st *store.BreedState, catchTime int64) {
	if st == nil {
		return
	}
	dst := st.Eggs[:0]
	for _, e := range st.Eggs {
		if e.EggCatchTime == catchTime {
			continue
		}
		dst = append(dst, e)
	}
	st.Eggs = dst
}

func syncBreedHatchReady(st *store.BreedState) bool {
	if st == nil || st.EggCatchTime == 0 || st.EggID <= 0 || st.HatchState != 1 {
		return false
	}
	intimacy := st.Intimacy
	if intimacy < 1 {
		intimacy = 1
	}
	elapsed := time.Now().Unix() - st.EggCatchTime
	remain := breedEggHatchSec - elapsed
	if remain < 0 {
		remain = 0
	}
	st.HatchLeftTime = remain
	if remain <= 0 && intimacy >= 5 {
		st.HatchState = 2
		st.HatchLeftTime = 0
		return true
	}
	return false
}

func (s *Server) loadBreed(uid int64) store.BreedState {
	st := store.BreedState{Intimacy: 1}
	if s.cfg.Store == nil {
		return st
	}
	got, err := s.cfg.Store.GetBreedState(uid)
	if err == nil {
		st = got
	}
	normalizeBreedEggs(&st)
	return st
}

func (s *Server) saveBreed(uid int64, st store.BreedState) {
	if s.cfg.Store == nil {
		return
	}
	normalizeBreedEggs(&st)
	_ = s.cfg.Store.SetBreedState(uid, st)
}

func buildBreedInfoBody(st store.BreedState, eggIDFallback int) []byte {
	normalizeBreedEggs(&st)
	syncBreedHatchReady(&st)

	maleID := uint32(st.MalePetID)
	femaleID := uint32(st.FemalePetID)
	maleCT := uint32(st.MaleCatchTime)
	femaleCT := uint32(st.FemaleCatchTime)
	eggID := uint32(st.EggID)
	if eggID == 0 && eggIDFallback > 0 {
		eggID = uint32(eggIDFallback)
	}

	pairOK := maleID != 0 && femaleID != 0 && maleCT != 0 && femaleCT != 0
	breedState := uint32(0)
	if pairOK {
		breedState = 2
	}

	intimacy := st.Intimacy
	if intimacy < 1 {
		intimacy = 1
	}
	if intimacy > 5 {
		intimacy = 5
	}

	hatchState := uint32(st.HatchState)
	hatchLeft := uint32(st.HatchLeftTime)
	if st.EggCatchTime != 0 && hatchState == 1 {
		elapsed := time.Now().Unix() - st.EggCatchTime
		remain := breedEggHatchSec - elapsed
		if remain < 0 {
			remain = 0
		}
		hatchLeft = uint32(remain)
		if remain <= 0 && intimacy >= 5 {
			hatchState = 2
			hatchLeft = 0
		}
	}
	if hatchState == 2 {
		hatchLeft = 0
	}

	cool := uint32(0)
	if st.EggCatchTime > 0 && breedEggProduceSec > 0 {
		elapsed := time.Now().Unix() - st.EggCatchTime
		remainCool := breedEggProduceSec - elapsed
		if remainCool > 0 {
			cool = uint32(remainCool)
		}
	}

	body := make([]byte, 44)
	binary.BigEndian.PutUint32(body[0:4], breedState)
	binary.BigEndian.PutUint32(body[4:8], cool)
	binary.BigEndian.PutUint32(body[8:12], cool)
	binary.BigEndian.PutUint32(body[12:16], maleCT)
	binary.BigEndian.PutUint32(body[16:20], maleID)
	binary.BigEndian.PutUint32(body[20:24], femaleCT)
	binary.BigEndian.PutUint32(body[24:28], femaleID)
	binary.BigEndian.PutUint32(body[28:32], hatchState)
	binary.BigEndian.PutUint32(body[32:36], hatchLeft)
	binary.BigEndian.PutUint32(body[36:40], eggID)
	binary.BigEndian.PutUint32(body[40:44], uint32(intimacy))
	return body
}

// handleBreedInfo CMD 2365：BreedInfo 11×u32。
func (s *Server) handleBreedInfo(c *Client, uid uint32) {
	st := s.loadBreed(int64(uid))
	eggFallback := 0
	if st.EggID == 0 && st.MalePetID > 0 && st.FemalePetID > 0 && s.cfg.Catalog != nil {
		eggFallback = s.cfg.Catalog.BreedEggID(st.MalePetID, st.FemalePetID)
	}
	if syncBreedHatchReady(&st) {
		s.saveBreed(int64(uid), st)
	}
	body := buildBreedInfoBody(st, eggFallback)
	s.send(c, 2365, uid, 0, body)
	log.Printf("[CMD] OK     %s UID=%d male=%d female=%d egg=%d hatch=%d int=%d",
		cmdname.Format(2365), uid, st.MalePetID, st.FemalePetID, st.EggID, st.HatchState, st.Intimacy)
}

// handleEggList CMD 2367：count + [owner+eggCatchTime+eggID]*n。
func (s *Server) handleEggList(c *Client, uid uint32) {
	st := s.loadBreed(int64(uid))
	n := len(st.Eggs)
	body := make([]byte, 4+n*12)
	binary.BigEndian.PutUint32(body[0:4], uint32(n))
	for i, e := range st.Eggs {
		off := 4 + i*12
		binary.BigEndian.PutUint32(body[off:off+4], uid)
		binary.BigEndian.PutUint32(body[off+4:off+8], uint32(e.EggCatchTime))
		binary.BigEndian.PutUint32(body[off+8:off+12], uint32(e.EggID))
	}
	s.send(c, 2367, uid, 0, body)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(2367), uid, n)
}

// handleBreedPet CMD 2364：male.catchTime → 可匹配雌性 catchTime 列表。
func (s *Server) handleBreedPet(c *Client, uid uint32, body []byte) {
	empty := func() {
		out := make([]byte, 4)
		s.send(c, 2364, uid, 0, out)
	}
	if s.cfg.Store == nil || s.cfg.Catalog == nil || len(body) < 4 {
		empty()
		return
	}
	maleCT := int64(binary.BigEndian.Uint32(body[0:4]))
	male, err := s.cfg.Store.GetPetByCatchTime(int64(uid), maleCT)
	if err != nil || male == nil {
		empty()
		return
	}
	if s.cfg.Catalog.PetGender(male.PetID) != 1 {
		empty()
		return
	}
	bag, err := s.cfg.Store.ListBagPets(int64(uid))
	if err != nil {
		empty()
		return
	}
	females := make([]uint32, 0)
	for _, p := range bag {
		if p.Level < 50 {
			continue
		}
		if s.cfg.Catalog.PetGender(p.PetID) != 2 {
			continue
		}
		if s.cfg.Catalog.BreedEggID(male.PetID, p.PetID) == 0 {
			continue
		}
		females = append(females, uint32(p.CatchTime))
	}
	out := make([]byte, 4+len(females)*4)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(females)))
	for i, ct := range females {
		binary.BigEndian.PutUint32(out[4+i*4:8+i*4], ct)
	}
	s.send(c, 2364, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d male=%d females=%d", cmdname.Format(2364), uid, male.PetID, len(females))
}

// handleStartBreed CMD 2374：maleCT+femaleCT → 生蛋入列表。
func (s *Server) handleStartBreed(c *Client, uid uint32, body []byte) {
	s.send(c, 2374, uid, 0, nil)
	if s.cfg.Store == nil || s.cfg.Catalog == nil || len(body) < 8 {
		return
	}
	maleCT := int64(binary.BigEndian.Uint32(body[0:4]))
	femaleCT := int64(binary.BigEndian.Uint32(body[4:8]))
	male, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), maleCT)
	female, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), femaleCT)
	if male == nil || female == nil {
		return
	}
	if s.cfg.Catalog.PetGender(male.PetID) != 1 || s.cfg.Catalog.PetGender(female.PetID) != 2 {
		return
	}
	eggID := s.cfg.Catalog.BreedEggID(male.PetID, female.PetID)
	if eggID == 0 {
		log.Printf("[CMD] WARN  %s UID=%d no egg formula %d×%d", cmdname.Format(2374), uid, male.PetID, female.PetID)
		return
	}
	now := time.Now().Unix()
	st := s.loadBreed(int64(uid))
	st.MalePetID = male.PetID
	st.MaleCatchTime = maleCT
	st.FemalePetID = female.PetID
	st.FemaleCatchTime = femaleCT
	st.Eggs = append(st.Eggs, store.BreedEggEntry{EggID: eggID, EggCatchTime: now})
	if st.EggID == 0 || st.EggCatchTime == 0 {
		st.EggID = eggID
		st.EggCatchTime = now
	}
	if st.HatchState != 1 {
		st.HatchState = 0
		st.HatchLeftTime = 0
		st.Intimacy = 1
	}
	s.saveBreed(int64(uid), st)
	log.Printf("[CMD] OK     %s UID=%d egg=%d male=%d female=%d", cmdname.Format(2374), uid, eggID, male.PetID, female.PetID)
}

// handleStartHatch CMD 2368：owner+eggCatchTime → 开始孵化。
func (s *Server) handleStartHatch(c *Client, uid uint32, body []byte) {
	s.send(c, 2368, uid, 0, nil)
	if len(body) < 8 {
		return
	}
	eggCT := int64(binary.BigEndian.Uint32(body[4:8]))
	st := s.loadBreed(int64(uid))
	selected, ok := findBreedEgg(st.Eggs, eggCT)
	if !ok {
		return
	}
	if st.HatchState == 1 && st.EggCatchTime != 0 && st.EggCatchTime != eggCT {
		return
	}
	st.EggID = selected.EggID
	st.EggCatchTime = selected.EggCatchTime
	st.HatchState = 1
	st.Intimacy = 1
	elapsed := time.Now().Unix() - st.EggCatchTime
	remain := breedEggHatchSec - elapsed
	if remain < 0 {
		remain = 0
	}
	st.HatchLeftTime = remain
	s.saveBreed(int64(uid), st)
	log.Printf("[CMD] OK     %s UID=%d egg=%d catch=%d", cmdname.Format(2368), uid, st.EggID, eggCT)
}

// handleEffectHatch CMD 2369：互动 +1 亲密度，应答 intimacy(4)。
func (s *Server) handleEffectHatch(c *Client, uid uint32, body []byte) {
	st := s.loadBreed(int64(uid))
	intimacy := st.Intimacy
	if intimacy < 1 {
		intimacy = 1
	}
	intimacy++
	if intimacy > 5 {
		intimacy = 5
	}
	st.Intimacy = intimacy
	elapsed := time.Now().Unix() - st.EggCatchTime
	remain := breedEggHatchSec - elapsed
	if remain < 0 {
		remain = 0
	}
	st.HatchLeftTime = remain
	if intimacy >= 5 && remain <= 0 {
		st.HatchState = 2
		st.HatchLeftTime = 0
	} else if st.HatchState != 2 {
		st.HatchState = 1
	}
	s.saveBreed(int64(uid), st)
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(intimacy))
	s.send(c, 2369, uid, 0, out)
	_ = body
	log.Printf("[CMD] OK     %s UID=%d intimacy=%d", cmdname.Format(2369), uid, intimacy)
}

// handleGetHatchPet CMD 2370：领取 → petID+catchTime；再推 2365。
func (s *Server) handleGetHatchPet(c *Client, uid uint32) {
	empty := make([]byte, 8)
	st := s.loadBreed(int64(uid))
	if syncBreedHatchReady(&st) {
		s.saveBreed(int64(uid), st)
	}
	if st.HatchState != 2 || st.EggID <= 0 || st.EggCatchTime <= 0 || s.cfg.Catalog == nil || s.cfg.Store == nil {
		s.send(c, 2370, uid, 0, empty)
		return
	}
	selected, ok := findBreedEgg(st.Eggs, st.EggCatchTime)
	if !ok || selected.EggID != st.EggID {
		st.HatchState = 0
		st.HatchLeftTime = 0
		st.EggID = 0
		st.EggCatchTime = 0
		st.Intimacy = 1
		s.saveBreed(int64(uid), st)
		s.send(c, 2370, uid, 0, empty)
		return
	}
	petID := s.cfg.Catalog.EggOutputPetID(selected.EggID)
	if petID == 0 {
		s.send(c, 2370, uid, 0, empty)
		return
	}
	claimedCT := st.EggCatchTime
	removeBreedEgg(&st, claimedCT)
	st.HatchState = 0
	st.HatchLeftTime = 0
	st.EggID = 0
	st.EggCatchTime = 0
	st.Intimacy = 1
	s.saveBreed(int64(uid), st)

	name := s.cfg.Catalog.PetNameOf(petID)
	if name == "" {
		name = "精灵"
	}
	skills := s.defaultSkillsForPet(petID)
	dv := rand.Intn(32)
	nature := rand.Intn(25)
	catch, err := s.cfg.Store.GrantPet(int64(uid), petID, name, 1, dv, nature, skills)
	if err != nil {
		log.Printf("[CMD] FAIL   %s UID=%d GrantPet: %v", cmdname.Format(2370), uid, err)
		s.send(c, 2370, uid, 0, empty)
		return
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(petID))
	binary.BigEndian.PutUint32(out[4:8], uint32(catch))
	s.send(c, 2370, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d pet=%d catch=%d", cmdname.Format(2370), uid, petID, catch)

	// 推背包 2301
	if p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), catch); err == nil && p != nil {
		s.send(c, 2301, uid, 0, buildPetInfo(p))
	}
	s.handleBreedInfo(c, uid)
}

func (s *Server) defaultSkillsForPet(petID int) []int {
	if s.cfg.Catalog == nil {
		return []int{10001}
	}
	base := s.cfg.Catalog.PetBase(petID)
	out := make([]int, 0, 4)
	if base != nil {
		for _, m := range base.LearnableMoves {
			if m.ID > 0 && m.Level <= 1 {
				out = append(out, m.ID)
			}
			if len(out) >= 4 {
				break
			}
		}
		if len(out) == 0 {
			for _, m := range base.LearnableMoves {
				if m.ID > 0 {
					out = append(out, m.ID)
				}
				if len(out) >= 4 {
					break
				}
			}
		}
	}
	if len(out) == 0 {
		out = []int{10001}
	}
	return out
}
