package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

// 阿特莱斯空间奖励（CMD 2106）：MapProcess_316 小游戏 / 303–304 宝箱 / MazeController。
// 包体：type(u32)。应答 AresiaSpacePrize = bonusID + petID + captureTm + n + n×(itemID,cnt)。

var atresiaPrizeByType = map[uint32]struct {
	ItemID uint32
	Count  uint32
}{
	1: {ItemID: 300003, Count: 1}, // 宝箱：高级胶囊
	2: {ItemID: 300003, Count: 2}, // 迷宫宝箱
	3: {ItemID: 400028, Count: 2}, // 训练小游戏
	4: {ItemID: 400028, Count: 3},
	5: {ItemID: 300001, Count: 2}, // 初级胶囊 / game_3
}

func (s *Server) handlePrizeOfAtresiaSpace(c *Client, uid uint32, body []byte) {
	typ := uint32(0)
	if len(body) >= 4 {
		typ = binary.BigEndian.Uint32(body[0:4])
	}
	prize, ok := atresiaPrizeByType[typ]
	if !ok || prize.ItemID == 0 || prize.Count == 0 {
		s.send(c, 2106, uid, 0, buildAresiaSpacePrize(0, 0, 0, nil))
		log.Printf("[CMD] OK     %s UID=%d type=%d (empty)", cmdname.Format(2106), uid, typ)
		return
	}
	key := "atresia:" + itoaU32(typ)
	if !s.tryMarkDaily(int64(uid), key) {
		s.send(c, 2106, uid, 0, buildAresiaSpacePrize(0, 0, 0, nil))
		log.Printf("[CMD] OK     %s UID=%d type=%d daily-capped", cmdname.Format(2106), uid, typ)
		return
	}
	if s.cfg.Store != nil {
		_ = s.cfg.Store.AddItem(int64(uid), int(prize.ItemID), int(prize.Count))
	}
	items := [][2]uint32{{prize.ItemID, prize.Count}}
	s.send(c, 2106, uid, 0, buildAresiaSpacePrize(typ, 0, 0, items))
	log.Printf("[CMD] OK     %s UID=%d type=%d item=%d x%d", cmdname.Format(2106), uid, typ, prize.ItemID, prize.Count)
}

func buildAresiaSpacePrize(bonusID, petID, captureTm uint32, items [][2]uint32) []byte {
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], bonusID)
	binary.BigEndian.PutUint32(out[4:8], petID)
	binary.BigEndian.PutUint32(out[8:12], captureTm)
	binary.BigEndian.PutUint32(out[12:16], uint32(len(items)))
	for _, it := range items {
		b := make([]byte, 8)
		binary.BigEndian.PutUint32(b[0:4], it[0])
		binary.BigEndian.PutUint32(b[4:8], it[1])
		out = append(out, b...)
	}
	return out
}
