package gameserver

import (
	"encoding/binary"
	"log"
	"strconv"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// 基地精灵恢复仓 Ext_2 → PetManager.cureAll → 2306（房间 TCP 同样走本 handler）。
// 客户端 ACK 后本地满血并 coins-=50；VIP 仅跳过确认框，仍扣豆。
const (
	petCureAllCost = 50
	petCureOneCost = 20
)

// cureAllBagPets 清空背包精灵 HP 记忆（满血）；扣豆用 TrySpendCoins，余额不足仍治疗并 ACK（对齐客户端先确认再发包）。
func (s *Server) cureAllBagPets(uid int64, cost int) (bagN int, spent bool) {
	if s.cfg.Store == nil {
		return 0, false
	}
	bag, _ := s.cfg.Store.ListBagPets(uid)
	for i := range bag {
		s.forgetPetHP(uid, uint32(bag[i].CatchTime))
	}
	if cost > 0 {
		if _, ok, err := s.cfg.Store.TrySpendCoins(uid, cost); err == nil && ok {
			spent = true
		}
	}
	return len(bag), spent
}

// handlePetCureAll CMD 2306：背包全员治疗。空 ACK。
// 来源：恢复仓 Ext_2 / Nono 快捷栏 / PetManager.cureAll（基地 MapManager.type=ROOM 时走房间连接）。
func (s *Server) handlePetCureAll(c *Client, uid uint32) {
	n, spent := s.cureAllBagPets(int64(uid), petCureAllCost)
	s.send(c, 2306, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d bag=%d spent=%v room=%v",
		cmdname.Format(2306), uid, n, spent, c != nil && c.IsRoom)
}

// handlePetOneCure CMD 2310：单宠治疗。请求/应答 catchTime(4)。
func (s *Server) handlePetOneCure(c *Client, uid uint32, body []byte) {
	catch := uint32(0)
	if len(body) >= 4 {
		catch = binary.BigEndian.Uint32(body[0:4])
	}
	spent := false
	if s.cfg.Store != nil && catch > 0 {
		s.forgetPetHP(int64(uid), catch)
		if _, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), petCureOneCost); err == nil && ok {
			spent = true
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, catch)
	s.send(c, 2310, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d spent=%v room=%v",
		cmdname.Format(2310), uid, catch, spent, c != nil && c.IsRoom)
}

// handlePetRoweiList CMD 2320：放生仓列表。PetListInfo(无 elite)=12B/只。
func (s *Server) handlePetRoweiList(c *Client, uid uint32) {
	pets := []store.Pet{}
	if s.cfg.Store != nil {
		pets, _ = s.cfg.Store.ListRoweiPets(int64(uid))
	}
	out := buildRoweiListBody(pets)
	s.send(c, 2320, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d count=%d", cmdname.Format(2320), uid, len(pets))
}

// handlePetRowei CMD 2321：仓库 → 放生仓。请求 petID+catchTime；应答空。
// 尼尔号：按公式发回收赛尔豆（FreeForbidden 除外）。
func (s *Server) handlePetRowei(c *Client, uid uint32, body []byte) {
	petID, catch := uint32(0), uint32(0)
	if len(body) >= 8 {
		petID = binary.BigEndian.Uint32(body[0:4])
		catch = binary.BigEndian.Uint32(body[4:8])
	}
	ok := false
	coins, honor := 0, 0
	if s.cfg.Store != nil && catch > 0 {
		var dv, lv, pid int
		if p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch)); err == nil && p != nil {
			dv, lv, pid = p.DV, p.Level, p.PetID
			if petID == 0 {
				petID = uint32(pid)
			}
		}
		if err := s.cfg.Store.MovePetToRowei(int64(uid), int64(catch)); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d pet=%d catch=%d: %v",
				cmdname.Format(2321), uid, petID, catch, err)
		} else {
			ok = true
			coins, honor = s.grantPetRecycleReward(int64(uid), pid, dv, lv)
			if coins > 0 {
				s.sendAlert(int64(uid), "回收获得赛尔豆 "+strconv.Itoa(coins))
			} else if !s.hasActiveSuperNono(int64(uid)) && !s.isPetRecycleForbidden(pid) {
				s.sendAlert(int64(uid), "超能NoNo已过期，放生不获得赛尔豆")
			}
			if honor > 0 {
				s.sendAlert(int64(uid), "今日首次回收，荣誉+"+strconv.Itoa(honor))
			}
		}
	}
	s.send(c, 2321, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d pet=%d catch=%d ok=%v coins=%d honor=%d",
		cmdname.Format(2321), uid, petID, catch, ok, coins, honor)
}

// handlePetRetrieve CMD 2322：放生仓 → 仓库。请求 catchTime。
func (s *Server) handlePetRetrieve(c *Client, uid uint32, body []byte) {
	catch := uint32(0)
	if len(body) >= 4 {
		catch = binary.BigEndian.Uint32(body[0:4])
	}
	ok := false
	if s.cfg.Store != nil && catch > 0 {
		if err := s.cfg.Store.RetrievePetFromRowei(int64(uid), int64(catch)); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d catch=%d: %v", cmdname.Format(2322), uid, catch, err)
		} else {
			ok = true
		}
	}
	s.send(c, 2322, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d catch=%d ok=%v", cmdname.Format(2322), uid, catch, ok)
}

func buildRoweiListBody(pets []store.Pet) []byte {
	// 与 2303 不同：2320 用 PetListInfo(param2=false) → 无 isElite，12 字节/只
	out := make([]byte, 4+len(pets)*12)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(pets)))
	off := 4
	for i := range pets {
		binary.BigEndian.PutUint32(out[off:off+4], uint32(pets[i].PetID))
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(pets[i].CatchTime))
		binary.BigEndian.PutUint32(out[off+8:off+12], petSkinID(&pets[i]))
		off += 12
	}
	return out
}
