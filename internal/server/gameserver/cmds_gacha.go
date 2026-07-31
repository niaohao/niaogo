package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"time"

	"niaohao/server/internal/cmdname"
)

const gachaTicketID = 400501

// handleGacha CMD 3201 精灵扭蛋机：扣 400501×次数，回包 coins+pet+items。
// 请求 btnIndex(4)：1=单抽 2=五连 3=十连。
func (s *Server) handleGacha(c *Client, uid uint32, body []byte) {
	fail := func() {
		s.send(c, 3201, uid, 0, nil)
	}
	if s.cfg.Store == nil {
		fail()
		return
	}
	times := 1
	if len(body) >= 4 {
		switch binary.BigEndian.Uint32(body[0:4]) {
		case 2:
			times = 5
		case 3:
			times = 10
		default:
			times = 1
		}
	}
	n, _ := s.cfg.Store.GetItemCount(int64(uid), gachaTicketID)
	if n < times {
		fail()
		log.Printf("[CMD] OK     %s UID=%d no ticket need=%d have=%d", cmdname.Format(3201), uid, times, n)
		return
	}
	if err := s.cfg.Store.ConsumeItem(int64(uid), gachaTicketID, times); err != nil {
		fail()
		return
	}

	rewards := make(map[int]int)
	for i := 0; i < times; i++ {
		itemID, count := s.rollGachaOnce(int64(uid))
		if itemID > 0 && count > 0 {
			rewards[itemID] += count
		}
	}
	if len(rewards) == 0 {
		fail()
		return
	}

	coins, _ := s.cfg.Store.GetCoins(int64(uid))
	if coins < 0 {
		coins = 0
	}
	out := make([]byte, 0, 16+len(rewards)*8)
	tmp := make([]byte, 4)
	binary.BigEndian.PutUint32(tmp, uint32(coins))
	out = append(out, tmp...)
	binary.BigEndian.PutUint32(tmp, 0) // petID
	out = append(out, tmp...)
	binary.BigEndian.PutUint32(tmp, 0) // catchTime
	out = append(out, tmp...)
	binary.BigEndian.PutUint32(tmp, uint32(len(rewards)))
	out = append(out, tmp...)
	for id, cnt := range rewards {
		binary.BigEndian.PutUint32(tmp, uint32(id))
		out = append(out, tmp...)
		binary.BigEndian.PutUint32(tmp, uint32(cnt))
		out = append(out, tmp...)
	}
	s.send(c, 3201, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d times=%d rewards=%d", cmdname.Format(3201), uid, times, len(rewards))
}

// rollGachaOnce 权重抽奖并发放；返回用于提示的 itemID/count（货币用 1/5/3）。
func (s *Server) rollGachaOnce(uid int64) (itemID, count int) {
	type entry struct {
		w    int
		kind string
		id   int
		n    int
	}
	pool := []entry{
		{5, "coins", 1, 5000},
		{5, "coins", 1, 10000},
		{5, "gold", 5, 5},
		{2, "gold", 5, 10},
		{10, "exp", 3, 10000},
		{5, "exp", 3, 20000},
		{2, "exp", 3, 50000},
		{1, "exp", 3, 100000},
		{3, "item", 300790, 1},
		{5, "item", 300006, 1},
		{5, "item", 300009, 1},
		{3, "item", 300053, 1},
		{7, "item", 300054, 1},
		{5, "item", 300651, 1},
		{5, "item", 300026, 1},
		{5, "item", 300025, 1},
		{5, "item", 400501, 1},
		{5, "item", 400501, 2},
		{5, "item", 400501, 3},
		{5, "item", 300024, 1},
		{5, "item", 300004, 5},
		{5, "item", 300001, 5},
		{5, "item", 300011, 5},
		{5, "item", 300152, 1},
		{5, "item", 300153, 1},
	}
	total := 0
	for _, e := range pool {
		total += e.w
	}
	if total <= 0 {
		return 0, 0
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	pick := r.Intn(total)
	cur := 0
	var chosen entry
	for _, e := range pool {
		cur += e.w
		if pick < cur {
			chosen = e
			break
		}
	}
	switch chosen.kind {
	case "coins":
		_ = s.cfg.Store.AddCoins(uid, chosen.n)
		return chosen.id, chosen.n
	case "gold":
		_ = s.cfg.Store.AddGold(uid, chosen.n)
		return chosen.id, chosen.n
	case "exp":
		_, _ = s.cfg.Store.AddExpPool(uid, chosen.n)
		return chosen.id, chosen.n
	default:
		if err := s.cfg.Store.AddItem(uid, chosen.id, chosen.n); err != nil {
			return 0, 0
		}
		return chosen.id, chosen.n
	}
}
