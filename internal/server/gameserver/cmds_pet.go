package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

func (s *Server) handlePetRelease(c *Client, uid uint32, body []byte) {
	// 2304 PET_RELEASE → PetTakeOutInfo
	// flag=0 背包→仓库；flag=1 仓库→背包（或已在背包仅取 PetInfo）
	catchID := uint32(0)
	reqFlag := uint32(0)
	if len(body) >= 4 {
		catchID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		reqFlag = binary.BigEndian.Uint32(body[4:8])
	}

	var petBody []byte
	respFlag := uint32(0)
	firstPet := uint32(0)

	if s.cfg.Store != nil {
		_, _ = s.cfg.Store.NormalizeBagOverflow(int64(uid))
		switch reqFlag {
		case 0: // 背包 → 仓库
			ft, ok, err := s.cfg.Store.MovePetToStorage(int64(uid), int64(catchID))
			if err != nil {
				log.Printf("[CMD] WARN  %s UID=%d MovePetToStorage: %v", cmdname.Format(2304), uid, err)
			}
			if ok {
				firstPet = uint32(ft)
				respFlag = 0
			} else if bag, _ := s.cfg.Store.ListBagPets(int64(uid)); len(bag) > 0 {
				firstPet = uint32(bag[0].CatchTime)
			}
		case 1: // 仓库 → 背包 / 取信息
			p, ok, err := s.cfg.Store.MovePetToBag(int64(uid), int64(catchID))
			if err != nil {
				log.Printf("[CMD] WARN  %s UID=%d MovePetToBag: %v", cmdname.Format(2304), uid, err)
			}
			if ok && p != nil {
				respFlag = 1
				petBody = buildPetInfo(p)
			}
			if bag, _ := s.cfg.Store.ListBagPets(int64(uid)); len(bag) > 0 {
				firstPet = uint32(bag[0].CatchTime)
			}
		default:
			if bag, _ := s.cfg.Store.ListBagPets(int64(uid)); len(bag) > 0 {
				firstPet = uint32(bag[0].CatchTime)
			}
		}
	}

	out := buildPetTakeOutInfo(firstPet, respFlag, petBody)
	s.send(c, 2304, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d reqFlag=%d respFlag=%d petBody=%d first=%d",
		cmdname.Format(2304), uid, catchID, reqFlag, respFlag, len(petBody), firstPet)
}

func (s *Server) handleGetPetInfo(c *Client, uid uint32, body []byte) {
	// 2301 GET_PET_INFO：请求 catchTime(4)，响应完整 PetInfo
	catchID := uint32(0)
	if len(body) >= 4 {
		catchID = binary.BigEndian.Uint32(body[0:4])
	}
	var out []byte
	if s.cfg.Store != nil {
		_, _ = s.cfg.Store.NormalizeBagOverflow(int64(uid))
		if p, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catchID)); p != nil {
			if !debugFightNoSkills && s.fillPetSkillsUpToFour(p) {
				_ = s.cfg.Store.UpsertPet(p)
			}
			out = buildPetInfo(p)
		} else if bag, _ := s.cfg.Store.ListBagPets(int64(uid)); len(bag) > 0 {
			p := &bag[0]
			if !debugFightNoSkills && s.fillPetSkillsUpToFour(p) {
				_ = s.cfg.Store.UpsertPet(p)
			}
			out = buildPetInfo(p)
		}
	}
	if out == nil {
		out = []byte{}
	}
	s.send(c, 2301, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d body=%d", cmdname.Format(2301), uid, catchID, len(out))
}

func (s *Server) handleGetPetList(c *Client, uid uint32, body []byte) {
	// 2303 GET_PET_LIST：仓库列表（本客户端 getStorageList 无 type，默认仓库）
	// PetListInfo(param2=true): id + catchTime + skinID + isElite
	listType := uint32(0)
	if len(body) >= 4 {
		listType = binary.BigEndian.Uint32(body[0:4])
	}
	pets := []store.Pet{}
	if s.cfg.Store != nil {
		_, _ = s.cfg.Store.NormalizeBagOverflow(int64(uid))
		if listType == 0 {
			pets, _ = s.cfg.Store.ListStoragePets(int64(uid))
		}
		// listType==1：部分路径当放生仓；正式放生仓走 2320
		if listType == 1 {
			pets, _ = s.cfg.Store.ListRoweiPets(int64(uid))
		}
	}
	out := buildPetListBody(pets)
	s.send(c, 2303, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d type=%d count=%d", cmdname.Format(2303), uid, listType, len(pets))
}

func (s *Server) handleSetDefaultPet(c *Client, uid uint32, body []byte) {
	// 2308 PET_DEFAULT：请求 catchTime(4)；应答 4 字节 0
	resp := make([]byte, 4)
	catchID := uint32(0)
	if len(body) >= 4 {
		catchID = binary.BigEndian.Uint32(body[0:4])
	}
	ok := false
	if s.cfg.Store != nil && catchID > 0 {
		if err := s.cfg.Store.SetDefaultPet(int64(uid), int64(catchID)); err != nil {
			log.Printf("[CMD] WARN  %s UID=%d SetDefaultPet catch=%d: %v",
				cmdname.Format(2308), uid, catchID, err)
		} else {
			ok = true
			// 登录/进战按 bag_pos 取首发；成功后确认 bag[0] 与请求一致
			if bag, err := s.cfg.Store.ListBagPets(int64(uid)); err == nil && len(bag) > 0 {
				if uint32(bag[0].CatchTime) != catchID {
					log.Printf("[CMD] WARN  %s UID=%d bag lead mismatch want=%d got=%d",
						cmdname.Format(2308), uid, catchID, bag[0].CatchTime)
				}
			}
		}
	}
	s.send(c, 2308, uid, 0, resp)
	log.Printf("[CMD] OK     %s UID=%d catch=%d synced=%v", cmdname.Format(2308), uid, catchID, ok)
}

func (s *Server) handlePetShow(c *Client, uid uint32, body []byte) {
	// 2305 PET_SHOW → PetShowInfo（本客户端无 shiny）：userID+catchTime+petID+flag+dv+skinID
	reqCatch := uint32(0)
	reqFlag := uint32(0)
	if len(body) >= 8 {
		reqCatch = binary.BigEndian.Uint32(body[0:4])
		reqFlag = binary.BigEndian.Uint32(body[4:8])
	}
	petID, catchTime, dv, skin := uint32(0), uint32(0), uint32(0), uint32(0)
	if s.cfg.Store != nil && reqCatch > 0 {
		if p, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(reqCatch)); p != nil {
			petID = uint32(p.PetID)
			catchTime = reqCatch
			dv = uint32(p.DV)
			skin = petSkinID(p)
		}
	}
	if reqFlag == 0 {
		// 取消跟随：仍回包，pet 字段可为 0
		if reqCatch == 0 {
			petID, catchTime, dv, skin = 0, 0, 0, 0
		}
	}
	out := make([]byte, 24)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], catchTime)
	binary.BigEndian.PutUint32(out[8:12], petID)
	binary.BigEndian.PutUint32(out[12:16], reqFlag)
	binary.BigEndian.PutUint32(out[16:20], dv)
	binary.BigEndian.PutUint32(out[20:24], skin)
	s.send(c, 2305, uid, 0, out)
	// 同图广播（他人可见跟随）
	s.broadcastToMap(c, 2305, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d pet=%d flag=%d",
		cmdname.Format(2305), uid, catchTime, petID, reqFlag)
}

func buildPetListBody(pets []store.Pet) []byte {
	out := make([]byte, 4+len(pets)*16)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(pets)))
	off := 4
	for i := range pets {
		elite := uint32(0)
		if pets[i].IsElite {
			elite = 1
		}
		binary.BigEndian.PutUint32(out[off:off+4], uint32(pets[i].PetID))
		binary.BigEndian.PutUint32(out[off+4:off+8], uint32(pets[i].CatchTime))
		binary.BigEndian.PutUint32(out[off+8:off+12], petSkinID(&pets[i]))
		binary.BigEndian.PutUint32(out[off+12:off+16], elite)
		off += 16
	}
	return out
}

// broadcastToMap 向同图其他玩家推送（不含自己；自己已由 send 回包）。
func (s *Server) broadcastToMap(from *Client, cmd int32, body []byte) {
	if from == nil || from.MapID == 0 {
		return
	}
	s.broadcastToMapID(from.MapID, from.UserID, cmd, body)
}

// broadcastToMapID 向指定地图除 excludeUID 外的在线玩家推包。
func (s *Server) broadcastToMapID(mapID int, excludeUID int64, cmd int32, body []byte) {
	if mapID == 0 {
		return
	}
	s.mu.Lock()
	m := s.mapUsers[mapID]
	targets := make([]*Client, 0)
	for _, c := range m {
		if c != nil && c.UserID != excludeUID {
			targets = append(targets, c)
		}
	}
	s.mu.Unlock()
	hdrUID := uint32(excludeUID)
	for _, c := range targets {
		uid := hdrUID
		if uid == 0 {
			uid = uint32(c.UserID)
		}
		s.send(c, cmd, uid, 0, body)
	}
}

// notifyMapLeave 向同图其他人推 2002（body=离开者 UID），对齐客户端 removeUser。
func (s *Server) notifyMapLeave(mapID int, leaveUID int64) {
	if mapID <= 0 || leaveUID <= 0 {
		return
	}
	body := make([]byte, 4)
	binary.BigEndian.PutUint32(body, uint32(leaveUID))
	s.broadcastToMapID(mapID, leaveUID, 2002, body)
}

// sendToUser 向指定在线 UID 推包（私聊等）。
func (s *Server) sendToUser(uid int64, cmd int32, body []byte) {
	s.mu.Lock()
	c := s.byUID[uid]
	s.mu.Unlock()
	if c == nil || !c.LoggedIn {
		return
	}
	s.send(c, cmd, uint32(uid), 0, body)
}
