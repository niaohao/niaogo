package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

const (
	petGeneRecastCost  = 5000
	petGeneSubClass    = 119 // 闪光尼尔/菲斯利/艾菲亚
	petGeneSubKaruyeke = 315 // 卡鲁耶克（最高形态）
)

// handlePetGeneRecast CMD 70000 PET_GENE_RECAST：实验室基因重组器。
// 对齐《尼尔号游玩须知》：闪尼补 1 颗宝石（右→左）并重随性格；卡鲁直升 31 保性格；满个体闪尼只洗性格。
// 请求 mainCatch(4)+subCatch(4)；失败 flag=0；成功 flag+petId+catchTime。
func (s *Server) handlePetGeneRecast(c *Client, uid uint32, body []byte) {
	fail := func(why string) {
		out := make([]byte, 4)
		s.send(c, 70000, uid, 0, out)
		log.Printf("[CMD] OK     %s UID=%d fail: %s", cmdname.Format(70000), uid, why)
	}
	if len(body) < 8 || s.cfg.Store == nil {
		fail("bad body")
		return
	}
	mainCT := int64(binary.BigEndian.Uint32(body[0:4]))
	subCT := int64(binary.BigEndian.Uint32(body[4:8]))
	if mainCT == 0 || subCT == 0 || mainCT == subCT {
		fail("bad catch")
		return
	}
	main, err := s.cfg.Store.GetPetByCatchTime(int64(uid), mainCT)
	if err != nil || main == nil || !main.InBag {
		fail("main miss/not bag")
		return
	}
	sub, err := s.cfg.Store.GetPetByCatchTime(int64(uid), subCT)
	if err != nil || sub == nil || !sub.InBag {
		fail("sub miss/not bag")
		return
	}
	if !s.petGeneMainOK(main) {
		fail("main not final form")
		return
	}
	if !s.petGeneSubOK(sub) {
		fail("sub not gene donor")
		return
	}

	if _, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), petGeneRecastCost); err != nil || !ok {
		fail("no coins")
		return
	}

	newDV, nature := applyGeneRecast(main.DV, main.Nature, sub)
	catch, err := s.grantNewPetFull(int64(uid), main.PetID, 1, newDV, nature, -1)
	if err != nil {
		fail("grant: " + err.Error())
		return
	}
	_ = s.cfg.Store.DeletePet(int64(uid), mainCT)
	_ = s.cfg.Store.DeletePet(int64(uid), subCT)

	out := make([]byte, 12)
	binary.BigEndian.PutUint32(out[0:4], 1)
	binary.BigEndian.PutUint32(out[4:8], uint32(main.PetID))
	binary.BigEndian.PutUint32(out[8:12], uint32(catch))
	s.send(c, 70000, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d ok main=%d sub=%d -> pet=%d catch=%d dv=%d/%d->%d nature=%d",
		cmdname.Format(70000), uid, mainCT, subCT, main.PetID, catch, main.DV, sub.DV, newDV, nature)
}

func (s *Server) petGeneMainOK(p *store.Pet) bool {
	if p == nil || s.cfg.Catalog == nil {
		return false
	}
	d := s.cfg.Catalog.PetBase(p.PetID)
	if d == nil {
		return false
	}
	return d.EvolvesTo == 0
}

func (s *Server) petGeneSubOK(p *store.Pet) bool {
	if p == nil {
		return false
	}
	// 卡鲁耶克：须最高形态（315 EvolvesTo=0）
	if p.PetID == petGeneSubKaruyeke {
		return true
	}
	if s.cfg.Catalog == nil {
		return false
	}
	d := s.cfg.Catalog.PetBase(p.PetID)
	return d != nil && d.PetClass == petGeneSubClass
}

func isKaruyekeDonor(sub *store.Pet) bool {
	return sub != nil && sub.PetID == petGeneSubKaruyeke
}

// applyGeneRecast 尼尔号规则。
func applyGeneRecast(mainDV, mainNature int, sub *store.Pet) (dv, nature int) {
	mainDV &= 31
	subDV := 0
	if sub != nil {
		subDV = sub.DV & 31
	}
	if isKaruyekeDonor(sub) {
		return 31, clampNature(mainNature)
	}
	// 闪尼系：满个体只洗性格；否则从右往左补一颗副宠有、主宠没有的宝石，并重随性格
	if mainDV == 31 {
		return 31, rand.Intn(25)
	}
	dv = addOneGeneGemRightToLeft(mainDV, subDV)
	return dv, rand.Intn(25)
}

func clampNature(n int) int {
	if n < 0 || n > 24 {
		return rand.Intn(25)
	}
	return n
}

// addOneGeneGemRightToLeft 宝石位权重 1,2,4,8,16；优先补右侧（低位）。
func addOneGeneGemRightToLeft(mainDV, subDV int) int {
	for bit := 0; bit < 5; bit++ {
		mask := 1 << bit
		if subDV&mask != 0 && mainDV&mask == 0 {
			return (mainDV | mask) & 31
		}
	}
	return mainDV & 31
}
