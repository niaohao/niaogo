package gameserver

import (
	"bytes"
	"encoding/binary"

	"niaohao/server/internal/store"
)

// buildInformBody 对齐 InformInfo：104B。
func buildInformBody(informType, fromUID uint32, nick string, accept, serverID, mapType, mapID uint32, mapName string) []byte {
	body := make([]byte, 104)
	binary.BigEndian.PutUint32(body[0:4], informType)
	binary.BigEndian.PutUint32(body[4:8], fromUID)
	nb := []byte(nick)
	if len(nb) > 16 {
		nb = nb[:16]
	}
	copy(body[8:24], nb)
	binary.BigEndian.PutUint32(body[24:28], accept)
	binary.BigEndian.PutUint32(body[28:32], serverID)
	binary.BigEndian.PutUint32(body[32:36], mapType)
	binary.BigEndian.PutUint32(body[36:40], mapID)
	mn := []byte(mapName)
	if len(mn) > 64 {
		mn = mn[:64]
	}
	copy(body[40:104], mn)
	return body
}

func (s *Server) pushInform(toUID int64, informType, fromUID uint32, nick string, accept uint32) {
	body := buildInformBody(informType, fromUID, nick, accept, 1, 0, 0, "")
	s.sendToUser(toUID, 8001, body)
}

func buildRelationListBody(friends []store.FriendEntry, blacks []store.BlackEntry) []byte {
	out := make([]byte, 0, 8+len(friends)*8+len(blacks)*4)
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, uint32(len(friends)))
	out = append(out, tmp...)
	for _, f := range friends {
		binary.BigEndian.PutUint32(tmp, uint32(f.UserID))
		out = append(out, tmp...)
		binary.BigEndian.PutUint32(tmp, f.TimePoke)
		out = append(out, tmp...)
	}
	binary.BigEndian.PutUint32(tmp, uint32(len(blacks)))
	out = append(out, tmp...)
	for _, b := range blacks {
		binary.BigEndian.PutUint32(tmp, uint32(b.UserID))
		out = append(out, tmp...)
	}
	return out
}

func appendClothesBlock(buf *[]byte, ids []uint32) {
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, uint32(len(ids)))
	*buf = append(*buf, tmp...)
	for _, id := range ids {
		binary.BigEndian.PutUint32(tmp, id)
		*buf = append(*buf, tmp...)
		binary.BigEndian.PutUint32(tmp, 0) // level
		*buf = append(*buf, tmp...)
	}
}

func appendClothesBlockFromBuf(buf *bytes.Buffer, ids []uint32) {
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, uint32(len(ids)))
	buf.Write(tmp)
	for _, id := range ids {
		binary.BigEndian.PutUint32(tmp, id)
		buf.Write(tmp)
		binary.BigEndian.PutUint32(tmp, 0)
		buf.Write(tmp)
	}
}

func (s *Server) wornClothIDs(uid int64) []uint32 {
	if s.cfg.Store == nil {
		return nil
	}
	clothes, err := s.cfg.Store.ListWornClothes(uid)
	if err != nil || len(clothes) == 0 {
		return nil
	}
	ids := make([]uint32, 0, len(clothes))
	for _, w := range clothes {
		if w.ItemID > 0 {
			ids = append(ids, uint32(w.ItemID))
		}
	}
	return ids
}

func clothIDsToStore(ids []uint32) []store.WornCloth {
	out := make([]store.WornCloth, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		out = append(out, store.WornCloth{ItemID: int(id), Level: 0})
	}
	return out
}
