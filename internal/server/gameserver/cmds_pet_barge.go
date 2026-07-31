package gameserver

import (
	"encoding/binary"
	"log"
	"sort"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

// handlePetBargeList CMD 2309：请求 fromPetID(4)+toPetID(4)。
// 应答 monCount(4)+[monID(4)+enCnt(4)+isCatched(4)+isKilled(4)]*n
// MapProcess_61 查 239..239：isKillList 非空则解锁大形态相关流程。
func (s *Server) handlePetBargeList(c *Client, uid uint32, body []byte) {
	fromID, toID := uint32(0), uint32(0)
	if len(body) >= 8 {
		fromID = binary.BigEndian.Uint32(body[0:4])
		toID = binary.BigEndian.Uint32(body[4:8])
	} else if len(body) >= 4 {
		fromID = binary.BigEndian.Uint32(body[0:4])
		toID = fromID
	}
	if fromID > toID {
		fromID, toID = toID, fromID
	}

	owned := map[uint32]struct{}{}
	if s.cfg.Store != nil {
		add := func(pets []store.Pet) {
			for _, p := range pets {
				pid := uint32(p.PetID)
				if pid >= fromID && pid <= toID {
					owned[pid] = struct{}{}
				}
			}
		}
		if bag, err := s.cfg.Store.ListBagPets(int64(uid)); err == nil {
			add(bag)
		}
		if stg, err := s.cfg.Store.ListStoragePets(int64(uid)); err == nil {
			add(stg)
		}
	}

	ids := make([]uint32, 0, len(owned))
	for id := range owned {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]byte, 4+len(ids)*16)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(ids)))
	for i, petID := range ids {
		base := 4 + i*16
		binary.BigEndian.PutUint32(out[base:base+4], petID)
		binary.BigEndian.PutUint32(out[base+4:base+8], 1)  // enCnt
		binary.BigEndian.PutUint32(out[base+8:base+12], 1) // isCatched
		binary.BigEndian.PutUint32(out[base+12:base+16], 1) // isKilled
	}
	s.send(c, 2309, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d range=%d..%d n=%d", cmdname.Format(2309), uid, fromID, toID, len(ids))
}
