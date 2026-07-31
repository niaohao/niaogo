package gameserver

import (
	"encoding/binary"
	"log"
	"time"

	"niaohao/server/internal/store"
)

// handlePetHatch CMD 2315：放入精元。请求 itemID(4)；应答 result(4) 0=成功。
func (s *Server) handlePetHatch(c *Client, uid uint32, body []byte) {
	out := make([]byte, 4)
	fail := func() {
		binary.BigEndian.PutUint32(out[0:4], 1)
		s.send(c, 2315, uid, 0, out)
	}
	if s.cfg.Store == nil || s.cfg.Catalog == nil || len(body) < 4 {
		fail()
		return
	}
	itemID := int(binary.BigEndian.Uint32(body[0:4]))
	eb, ok := s.cfg.Catalog.EssenceBreedOf(itemID)
	if !ok || eb.BreedMonID <= 0 {
		fail()
		return
	}
	h, _ := s.cfg.Store.GetHatchState(int64(uid))
	if h.PetID > 0 {
		fail()
		return
	}
	n, _ := s.cfg.Store.GetItemCount(int64(uid), itemID)
	if n < 1 {
		fail()
		return
	}
	if err := s.cfg.Store.ConsumeItem(int64(uid), itemID, 1); err != nil {
		fail()
		return
	}
	dur := eb.BreedTime
	if dur <= 0 {
		dur = 1800
	}
	// 私服联调：精元孵化最短 60 秒
	if dur > 60 {
		dur = 60
	}
	_ = s.cfg.Store.SetHatchState(int64(uid), store.HatchState{
		PetID: eb.BreedMonID, ItemID: itemID, StartUnix: time.Now().Unix(), Duration: dur,
	})
	binary.BigEndian.PutUint32(out[0:4], 0)
	s.send(c, 2315, uid, 0, out)
	log.Printf("[CMD] OK     2315 PET_HATCH UID=%d item=%d -> pet=%d dur=%d", uid, itemID, eb.BreedMonID, dur)
}

// handlePetHatchGet CMD 2316：对齐 App_700002
// 应答 flag(4)+leftTime(4)+petID(4)+catchTime(4)。
func (s *Server) handlePetHatchGet(c *Client, uid uint32, body []byte) {
	out := make([]byte, 16)
	if s.cfg.Store == nil {
		s.send(c, 2316, uid, 0, out)
		return
	}
	h, err := s.cfg.Store.GetHatchState(int64(uid))
	if err != nil || h.PetID == 0 {
		s.send(c, 2316, uid, 0, out)
		return
	}
	now := time.Now().Unix()
	elapsed := now - h.StartUnix
	remain := int64(h.Duration) - elapsed
	if remain > 0 {
		binary.BigEndian.PutUint32(out[0:4], 1)
		binary.BigEndian.PutUint32(out[4:8], uint32(remain))
		binary.BigEndian.PutUint32(out[8:12], uint32(h.PetID))
		s.send(c, 2316, uid, 0, out)
		return
	}
	// 完成：先发宠，成功后再清状态（避免 clear-before-grant 丢精元）
	giveID := h.PetID
	if s.cfg.Catalog != nil {
		giveID = s.cfg.Catalog.EvolutionBaseForm(h.PetID)
	}
	catch, err := s.grantNewPet(int64(uid), giveID, 1)
	if err != nil {
		// 保留孵化态，客户端可重试领取
		binary.BigEndian.PutUint32(out[0:4], 1)
		binary.BigEndian.PutUint32(out[4:8], 0)
		binary.BigEndian.PutUint32(out[8:12], uint32(giveID))
		s.send(c, 2316, uid, 0, out)
		log.Printf("[CMD] FAIL  2316 PET_HATCH_GET UID=%d grant err=%v (state kept)", uid, err)
		return
	}
	_ = s.cfg.Store.ClearHatchState(int64(uid))
	// 先 8004 再 2316（对齐参考服与 BossCmdListener / App_700002）
	s.pushBossMonster8004(c, uid, uint32(giveID), uint32(catch))
	binary.BigEndian.PutUint32(out[0:4], 1)
	binary.BigEndian.PutUint32(out[4:8], 0) // leftTime=0 → 弹获得
	binary.BigEndian.PutUint32(out[8:12], uint32(giveID))
	binary.BigEndian.PutUint32(out[12:16], uint32(catch))
	s.send(c, 2316, uid, 0, out)
	log.Printf("[CMD] OK     2316 PET_HATCH_GET UID=%d pet=%d catch=%d", uid, giveID, catch)
}
