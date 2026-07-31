package gameserver

import (
	"encoding/binary"
	"log"
	"time"

	"niaohao/server/internal/cmdname"
)

// handleGetCurrentGoldNeoBean CMD 80007：优惠兑换所需赛尔豆数量。
func (s *Server) handleGetCurrentGoldNeoBean(c *Client, uid uint32) {
	need := goldPromoNeedCoins(0)
	if s.cfg.Store != nil {
		n, _ := s.cfg.Store.GetGoldPromoCount(int64(uid))
		need = goldPromoNeedCoins(n)
		if need <= 0 {
			// 优惠 5 次已用完：仍回末档价供 UI 展示，兑换时会拒绝 type0
			need = 250000
		}
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(need))
	s.send(c, 80007, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d need=%d", cmdname.Format(80007), uid, need)
}

// handleExchangeGoldNeoBean CMD 70004：赛尔豆兑金豆。
// 请求 type(4)：0=优惠兑30金豆；1/5/10=定额兑对应金豆。
// 应答 type(4)+goldAmount(4)。
func (s *Server) handleExchangeGoldNeoBean(c *Client, uid uint32, body []byte) {
	fail := func() {
		s.send(c, 70004, uid, 0, nil)
	}
	if s.cfg.Store == nil || len(body) < 4 {
		fail()
		return
	}
	typ := binary.BigEndian.Uint32(body[0:4])
	cost, goldGain, respType := 0, 0, typ
	switch typ {
	case 0:
		n, _ := s.cfg.Store.GetGoldPromoCount(int64(uid))
		cost = goldPromoNeedCoins(n)
		if cost <= 0 {
			s.sendAlert(int64(uid), "优惠兑换已达5次，请改用直接兑换（1万豆:1金豆）")
			fail()
			return
		}
		goldGain = 30
		// 客户端 onExchangeGold 对 1/2/3 本地扣豆；更高档推 1106 校正
		respType = uint32(goldPromoClientTier(n))
	case 1:
		cost, goldGain = 10000, 1
	case 5:
		cost, goldGain = 50000, 5
	case 10:
		cost, goldGain = 100000, 10
	default:
		fail()
		return
	}
	bal, ok, err := s.cfg.Store.TrySpendCoins(int64(uid), cost)
	if err != nil || !ok {
		fail()
		log.Printf("[CMD] OK     %s UID=%d fail coins need=%d bal=%d", cmdname.Format(70004), uid, cost, bal)
		return
	}
	if err := s.cfg.Store.AddGold(int64(uid), goldGain); err != nil {
		_ = s.cfg.Store.AddCoins(int64(uid), cost)
		fail()
		return
	}
	if typ == 0 {
		_, _ = s.cfg.Store.AddGoldPromoCount(int64(uid))
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], respType)
	binary.BigEndian.PutUint32(out[4:8], uint32(goldGain))
	s.send(c, 70004, uid, 0, out)
	s.pushGoldBalance1106(c, uid)
	log.Printf("[CMD] OK     %s UID=%d type=%d cost=%d gold+=%d", cmdname.Format(70004), uid, typ, cost, goldGain)
}

// goldPromoNeedCoins 第 n 次（0-based）优惠兑换所需赛尔豆。
// 攻略：5/10/15/20/25 万 → 30 金豆；超过 5 次返回 -1（改走定额 1万:1）。
func goldPromoNeedCoins(n int) int {
	if n < 0 {
		n = 0
	}
	if n >= 5 {
		return -1
	}
	return 50000 * (n + 1)
}

func goldPromoClientTier(n int) int {
	// 映射到客户端本地扣豆档 1/2/3（2万/4万/6万）；更高档仍回 3，并由 1106 校正余额
	switch {
	case n <= 0:
		return 1
	case n == 1:
		return 2
	default:
		return 3
	}
}

// userIsVip 超能 NoNo 未过期视为 VIP（米币/金豆商城折扣）。
func (s *Server) userIsVip(uid int64) bool {
	if s.cfg.Store == nil {
		return false
	}
	n, err := s.cfg.Store.GetNono(uid)
	if err != nil || n == nil {
		return false
	}
	if n.VipEndTime > time.Now().Unix() {
		return true
	}
	return n.SuperLevel > 0 && n.VipEndTime == 0
}
