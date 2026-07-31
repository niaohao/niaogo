package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
)

func (s *Server) handleItemList(c *Client, uid uint32, body []byte) {
	// 2605 ITEM_LIST：请求 a/b/c 三段；响应 count + [id+cnt+expire+level]*n
	var a, b, extra uint32
	if len(body) >= 12 {
		a = binary.BigEndian.Uint32(body[0:4])
		b = binary.BigEndian.Uint32(body[4:8])
		extra = binary.BigEndian.Uint32(body[8:12])
	}
	if a != 0 && b == 0 && extra == 0 {
		a, b, extra = 0, 0, 0
	}

	rows := make([]struct {
		id, cnt, exp, lv uint32
	}, 0)
	var equipLv map[int]int
	if s.cfg.Store != nil {
		equipLv, _ = s.cfg.Store.ListEquipLevels(int64(uid))
		items, err := s.cfg.Store.ListItemsInRange(int64(uid), int(a), int(b), int(extra))
		if err != nil {
			log.Printf("[CMD] WARN  %s UID=%d ListItems: %v", cmdname.Format(2605), uid, err)
		} else {
			for _, it := range items {
				lv := uint32(0)
				if equipLv != nil {
					if el, ok := equipLv[it.ItemID]; ok && el > 0 {
						lv = uint32(el)
					}
				}
				rows = append(rows, struct {
					id, cnt, exp, lv uint32
				}{uint32(it.ItemID), uint32(it.Count), uint32(it.ExpireTime), lv})
			}
		}
	}

	out := make([]byte, 4+len(rows)*16)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(rows)))
	off := 4
	for _, r := range rows {
		binary.BigEndian.PutUint32(out[off:off+4], r.id)
		binary.BigEndian.PutUint32(out[off+4:off+8], r.cnt)
		binary.BigEndian.PutUint32(out[off+8:off+12], r.exp)
		binary.BigEndian.PutUint32(out[off+12:off+16], r.lv)
		off += 16
	}
	s.send(c, 2605, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d range=%d-%d,%d count=%d",
		cmdname.Format(2605), uid, a, b, extra, len(rows))
}
