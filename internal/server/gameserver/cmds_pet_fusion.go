package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"strconv"
	"time"

	"niaohao/server/internal/store"
)

// 私服联调：元神赋形默认 60 秒（正式可改为 0 以使用 items.xml TransmuteTm）。
const soulBeadTransformSec int64 = 60

// 融合成功率（参考原码 handlePetFusion）。
const fusionSuccessRate = 80

func (s *Server) soulBeadRemainSec(bead *store.SoulBead) int64 {
	if bead == nil || bead.TransformStart <= 0 {
		return 0
	}
	dur := s.soulBeadTransformDuration(bead.ItemID)
	remain := dur - (time.Now().Unix() - bead.TransformStart)
	if remain < 0 {
		return 0
	}
	return remain
}

func (s *Server) soulBeadTransformDuration(itemID int) int64 {
	if soulBeadTransformSec > 0 {
		return soulBeadTransformSec
	}
	if s.cfg.Catalog != nil {
		if d, ok := s.cfg.Catalog.SoulBeadOf(itemID); ok && d.TransmuteTm > 0 {
			return int64(d.TransmuteTm)
		}
	}
	return 1800
}

func (s *Server) soulBeadAbsorbComplete(b *store.SoulBead) bool {
	if b == nil {
		return false
	}
	steps := 1
	if s.cfg.Catalog != nil {
		steps = s.cfg.Catalog.HatchAbsorbSteps(b.ItemID)
	}
	for i := 0; i < steps; i++ {
		if !b.Status[i] {
			return false
		}
	}
	return true
}

func (s *Server) pickBeigeResultPet() int {
	if s.cfg.Catalog != nil {
		if d, ok := s.cfg.Catalog.SoulBeadOf(wildBeigeBeadID); ok && len(d.TransmuteMon) > 0 {
			return d.TransmuteMon[rand.Intn(len(d.TransmuteMon))]
		}
	}
	return wildBeigePets[rand.Intn(len(wildBeigePets))]
}

func (s *Server) normalizeFusionResult(petID int) int {
	if petID <= 0 {
		return petID
	}
	if s.cfg.Catalog != nil {
		return s.cfg.Catalog.EvolutionBaseForm(petID)
	}
	return petID
}

func fusionDVFromPets(main, sub *store.Pet) int {
	if main == nil || sub == nil {
		return rand.Intn(32)
	}
	return calcFusionDV(main.DV, sub.DV)
}

func fusionTraitRecipeKey(matIDs []int) string {
	if len(matIDs) == 0 {
		return "empty"
	}
	ids := append([]int(nil), matIDs...)
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[j] < ids[i] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}
	key := ""
	for i, id := range ids {
		if i > 0 {
			key += ","
		}
		key += strconv.Itoa(id)
	}
	return key
}

func parseFusionMatsAndFlags(body []byte) (matIDs []int, keepSub, noDown bool, preferKeep, preferNoDown int) {
	if len(body) >= 24 {
		for i := 0; i < 4; i++ {
			start := 8 + 4*i
			id := int(binary.BigEndian.Uint32(body[start : start+4]))
			if id != 0 {
				matIDs = append(matIDs, id)
			}
		}
	}
	for _, id := range matIDs {
		if id == 300043 || id == 300137 {
			keepSub = true
		}
		if id == 300044 || id == 300140 {
			noDown = true
		}
	}
	if len(body) < 32 {
		return
	}
	flag1 := binary.BigEndian.Uint32(body[24:28])
	flag2 := binary.BigEndian.Uint32(body[28:32])
	useKeep := flag1 == 1 || flag2 == 1 || flag1 == 300043 || flag1 == 300137 || flag2 == 300043 || flag2 == 300137
	useNoDown := flag2 == 1 || flag1 == 300044 || flag1 == 300140 || flag2 == 300044 || flag2 == 300140
	if flag1 == 300043 || flag1 == 300137 {
		preferKeep = int(flag1)
	} else if flag2 == 300043 || flag2 == 300137 {
		preferKeep = int(flag2)
	}
	if flag1 == 300044 || flag1 == 300140 {
		preferNoDown = int(flag1)
	} else if flag2 == 300044 || flag2 == 300140 {
		preferNoDown = int(flag2)
	}
	if useKeep {
		keepSub = true
	}
	if useNoDown {
		noDown = true
	}
	return
}

func (s *Server) consumeFusionOptionalItem(uid int64, prefer int, alts ...int) bool {
	try := func(id int) bool {
		if id <= 0 {
			return false
		}
		n, _ := s.cfg.Store.GetItemCount(uid, id)
		if n <= 0 {
			return false
		}
		return s.cfg.Store.ConsumeItem(uid, id, 1) == nil
	}
	if try(prefer) {
		return true
	}
	for _, id := range alts {
		if try(id) {
			return true
		}
	}
	return false
}

func (s *Server) degradeSubPetLevel(uid int64, sub *store.Pet, down int) {
	if sub == nil || down <= 0 {
		return
	}
	sub.Level -= down
	if sub.Level < 1 {
		sub.Level = 1
	}
	sub.Exp = 0
	_ = s.cfg.Store.UpsertPet(sub)
	_ = uid
}

// handlePetFusion CMD 2351：mainCatch+subCatch[+mats][+flags] → 元神珠。
// 应答 obtainTime(4)+soulID(4)+starterCpTm(4)+costItemFlag(4)；失败 obtainTime=0。
func (s *Server) handlePetFusion(c *Client, uid uint32, body []byte) {
	out := make([]byte, 16)
	if s.cfg.Store == nil || len(body) < 8 {
		s.send(c, 2351, uid, 0, out)
		return
	}
	mainCT := int64(binary.BigEndian.Uint32(body[0:4]))
	subCT := int64(binary.BigEndian.Uint32(body[4:8]))
	main, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), mainCT)
	sub, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), subCT)
	if main == nil || sub == nil || mainCT == subCT {
		s.send(c, 2351, uid, 0, out)
		return
	}

	matIDs, keepSub, noDown, preferKeep, preferNoDown := parseFusionMatsAndFlags(body)
	for _, itemID := range matIDs {
		if itemID == 300043 || itemID == 300137 || itemID == 300044 || itemID == 300140 {
			continue
		}
		n, _ := s.cfg.Store.GetItemCount(int64(uid), itemID)
		if n > 0 {
			_ = s.cfg.Store.ConsumeItem(int64(uid), itemID, 1)
		}
	}
	// 材料位已带保留药则已标记；扩展标志位时再从背包扣
	if keepSub && !s.consumeFusionOptionalItem(int64(uid), preferKeep, 300043, 300137) {
		keepSub = false
	}
	if noDown && !s.consumeFusionOptionalItem(int64(uid), preferNoDown, 300044, 300140) {
		noDown = false
	}

	if rand.Intn(100) >= fusionSuccessRate {
		if !noDown {
			s.degradeSubPetLevel(int64(uid), sub, 5)
		}
		s.send(c, 2351, uid, 0, out)
		log.Printf("[CMD] FAIL  2351 PET_FUSION UID=%d main=%d sub=%d", uid, main.PetID, sub.PetID)
		return
	}

	beadID, resultID, ok := matchFusionFormula(main.PetID, sub.PetID)
	if !ok {
		beadID = wildBeigeBeadID
		resultID = s.pickBeigeResultPet()
	}
	resultID = s.normalizeFusionResult(resultID)
	if resultID <= 0 {
		s.send(c, 2351, uid, 0, out)
		return
	}

	obtain := uint32(time.Now().Unix())
	for i := 0; i < 5; i++ {
		if existing, _ := s.cfg.Store.GetSoulBead(int64(uid), obtain); existing == nil {
			break
		}
		obtain++
	}
	recipeKey := fusionTraitRecipeKey(matIDs)
	avoid := s.lastFusionTrait(int64(uid), recipeKey)
	trait := RollPetTraitAvoid(avoid)
	s.setFusionTrait(int64(uid), recipeKey, trait)
	bead := store.SoulBead{
		ObtainTime: obtain, ItemID: beadID, ResultPetID: resultID,
		ResultDV: fusionDVFromPets(main, sub), ResultNature: chooseFusionNature(main.Nature, sub.Nature),
		ResultTrait: trait,
	}
	if err := s.cfg.Store.UpsertSoulBead(int64(uid), bead); err != nil {
		s.send(c, 2351, uid, 0, out)
		return
	}
	_ = s.cfg.Store.AddItem(int64(uid), beadID, 1)
	_ = s.cfg.Store.DeletePet(int64(uid), mainCT)
	if !keepSub {
		_ = s.cfg.Store.DeletePet(int64(uid), subCT)
	}

	binary.BigEndian.PutUint32(out[0:4], obtain)
	binary.BigEndian.PutUint32(out[4:8], uint32(beadID))
	binary.BigEndian.PutUint32(out[8:12], uint32(mainCT))
	if keepSub {
		binary.BigEndian.PutUint32(out[12:16], 1)
	}
	s.send(c, 2351, uid, 0, out)
	log.Printf("[CMD] OK     2351 PET_FUSION UID=%d bead=%d resultPet=%d keepSub=%v", uid, beadID, resultID, keepSub)
}

// handleSoulBeadList CMD 2354：count + [obtain+itemID]*N
func (s *Server) handleSoulBeadList(c *Client, uid uint32) {
	if s.cfg.Store == nil {
		s.send(c, 2354, uid, 0, make([]byte, 4))
		return
	}
	list, err := s.cfg.Store.ListSoulBeads(int64(uid))
	if err != nil {
		s.send(c, 2354, uid, 0, make([]byte, 4))
		return
	}
	out := make([]byte, 4+len(list)*8)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(list)))
	for i, b := range list {
		off := 4 + i*8
		binary.BigEndian.PutUint32(out[off:off+4], b.ObtainTime)
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(b.ItemID))
	}
	s.send(c, 2354, uid, 0, out)
}

func (s *Server) handleGetSoulBeadBuf(c *Client, uid uint32, body []byte) {
	out := make([]byte, 24)
	var obtain uint32
	if len(body) >= 4 {
		obtain = binary.BigEndian.Uint32(body[0:4])
	}
	binary.BigEndian.PutUint32(out[0:4], obtain)
	if s.cfg.Store != nil && obtain != 0 {
		if b, _ := s.cfg.Store.GetSoulBead(int64(uid), obtain); b != nil {
			for i := 0; i < 20; i++ {
				if b.Status[i] {
					out[4+i] = 1
				}
			}
		}
	}
	s.send(c, 2352, uid, 0, out)
}

func (s *Server) handleSetSoulBeadBuf(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	if s.cfg.Store == nil || len(body) < 4 {
		s.send(c, 2353, uid, 0, out)
		return
	}
	obtain := binary.BigEndian.Uint32(body[0:4])
	b, _ := s.cfg.Store.GetSoulBead(int64(uid), obtain)
	if b != nil {
		raw := body[4:]
		for i := 0; i < 20 && i < len(raw); i++ {
			b.Status[i] = raw[i] != 0
		}
		_ = s.cfg.Store.UpsertSoulBead(int64(uid), *b)
	}
	s.send(c, 2353, uid, 0, out)
}

func (s *Server) handleGetSoulBeadStatus(c *Client, uid uint32, body []byte) {
	out := make([]byte, 12)
	if s.cfg.Store == nil {
		s.send(c, 2356, uid, 0, out)
		return
	}
	var bead *store.SoulBead
	if len(body) >= 4 {
		ot := binary.BigEndian.Uint32(body[0:4])
		if ot != 0 {
			bead, _ = s.cfg.Store.GetSoulBead(int64(uid), ot)
		}
	}
	if bead == nil {
		list, _ := s.cfg.Store.ListSoulBeads(int64(uid))
		for i := range list {
			if list[i].TransformStart > 0 {
				b := list[i]
				bead = &b
				break
			}
		}
	}
	if bead == nil || bead.TransformStart == 0 {
		s.send(c, 2356, uid, 0, out)
		return
	}
	binary.BigEndian.PutUint32(out[0:4], bead.ObtainTime)
	binary.BigEndian.PutUint32(out[4:8], uint32(bead.ItemID))
	binary.BigEndian.PutUint32(out[8:12], uint32(s.soulBeadRemainSec(bead)))
	s.send(c, 2356, uid, 0, out)
}

func (s *Server) handleTransformSoulBead(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	if s.cfg.Store == nil {
		s.send(c, 2357, uid, 0, out)
		return
	}
	var bead *store.SoulBead
	if len(body) >= 4 {
		ot := binary.BigEndian.Uint32(body[0:4])
		if ot != 0 {
			bead, _ = s.cfg.Store.GetSoulBead(int64(uid), ot)
		}
	}
	if bead == nil {
		list, _ := s.cfg.Store.ListSoulBeads(int64(uid))
		for i := range list {
			if list[i].TransformStart == 0 && s.soulBeadAbsorbComplete(&list[i]) {
				b := list[i]
				bead = &b
				break
			}
		}
	}
	if bead != nil && bead.TransformStart == 0 && s.soulBeadAbsorbComplete(bead) {
		bead.TransformStart = time.Now().Unix()
		_ = s.cfg.Store.UpsertSoulBead(int64(uid), *bead)
		log.Printf("[CMD] OK     2357 TRANSFORM_SOULBEAD UID=%d obtain=%d stepsOK bead=%d", uid, bead.ObtainTime, bead.ItemID)
	}
	s.send(c, 2357, uid, 0, out)
}

func (s *Server) handleSoulBeadToPet(c *Client, uid uint32, body []byte) {
	out := make([]byte, 12)
	if s.cfg.Store == nil {
		s.send(c, 2358, uid, 0, out)
		return
	}
	var bead *store.SoulBead
	if len(body) >= 4 {
		ot := binary.BigEndian.Uint32(body[0:4])
		if ot != 0 {
			bead, _ = s.cfg.Store.GetSoulBead(int64(uid), ot)
		}
	}
	now := time.Now().Unix()
	if bead == nil {
		list, _ := s.cfg.Store.ListSoulBeads(int64(uid))
		for i := range list {
			b := &list[i]
			if b.TransformStart > 0 && now-b.TransformStart >= s.soulBeadTransformDuration(b.ItemID) &&
				b.ResultPetID > 0 && s.soulBeadAbsorbComplete(b) {
				bead = b
				break
			}
		}
	}
	if bead == nil || bead.ResultPetID == 0 || bead.TransformStart == 0 {
		s.send(c, 2358, uid, 0, out)
		return
	}
	if now-bead.TransformStart < s.soulBeadTransformDuration(bead.ItemID) || !s.soulBeadAbsorbComplete(bead) {
		s.send(c, 2358, uid, 0, out)
		return
	}
	catch, err := s.grantNewPetFull(int64(uid), bead.ResultPetID, 1, bead.ResultDV, bead.ResultNature, bead.ResultTrait)
	if err != nil {
		s.send(c, 2358, uid, 0, out)
		return
	}
	_ = s.cfg.Store.DeleteSoulBead(int64(uid), bead.ObtainTime)
	_ = s.cfg.Store.ConsumeItem(int64(uid), bead.ItemID, 1)
	binary.BigEndian.PutUint32(out[0:4], uint32(bead.ResultPetID))
	binary.BigEndian.PutUint32(out[4:8], uint32(catch))
	s.send(c, 2358, uid, 0, out)
	s.pushBossMonster8004(c, uid, uint32(bead.ResultPetID), uint32(catch))
	log.Printf("[CMD] OK     2358 SOULBEAD_TO_PET UID=%d pet=%d nature=%d trait=%d", uid, bead.ResultPetID, bead.ResultNature, bead.ResultTrait)
}

func (s *Server) pushBossMonster8004(c *Client, uid, petID, catchTime uint32) {
	s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, petID, catchTime, 0, 0))
}
