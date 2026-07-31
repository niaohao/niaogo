package store

import (
	"database/sql"
	"time"
)

const (
	NonoStateFollowBit = 1 << 1
	NonoDefaultColor   = 0xFFFFFF
	NonoDefaultPower   = 100
	NonoDefaultMate    = 100
)

// Nono 用户 NoNo / 超能 NoNo 状态。
type Nono struct {
	UserID         int64
	HasNono        int
	Flag           int
	State          int
	Nick           string
	Color          int
	SuperNono      int
	Power          int
	Mate           int
	IQ             int
	AI             int
	Birth          int64
	ChargeTime     int
	FuncBits       []byte // 20 bytes
	SuperEnergy    int
	SuperLevel     int
	SuperStage     int
	SuperMonths    int
	VipValue       int
	AutoCharge     int
	VipEndTime     int64
	OpenGoldCharged bool
}

// DefaultNono 新建默认普通 NoNo（未开通）。
func DefaultNono(uid int64) *Nono {
	return &Nono{
		UserID:   uid,
		Flag:     1,
		Nick:     "NoNo",
		Color:    NonoDefaultColor,
		Power:    NonoDefaultPower,
		Mate:     NonoDefaultMate,
		FuncBits: make([]byte, 20),
	}
}

// GetNono 读取；无行返回 nil,nil。
func (s *sqlBackend) GetNono(uid int64) (*Nono, error) {
	n := &Nono{}
	var funcBits []byte
	var charged int
	err := s.db.QueryRow(`
SELECT user_id, has_nono, flag, state, nick, color, super_nono, power, mate, iq, ai,
       birth, charge_time, func_bits, super_energy, super_level, super_stage, super_months,
       vip_value, auto_charge, vip_end_time, open_gold_charged
FROM user_nono WHERE user_id=?`, uid).Scan(
		&n.UserID, &n.HasNono, &n.Flag, &n.State, &n.Nick, &n.Color, &n.SuperNono,
		&n.Power, &n.Mate, &n.IQ, &n.AI, &n.Birth, &n.ChargeTime, &funcBits,
		&n.SuperEnergy, &n.SuperLevel, &n.SuperStage, &n.SuperMonths,
		&n.VipValue, &n.AutoCharge, &n.VipEndTime, &charged)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n.FuncBits = normalizeFuncBits(funcBits)
	n.OpenGoldCharged = charged != 0
	return n, nil
}

// GetOrInitNono 无记录返回默认（不落库）。
func (s *sqlBackend) GetOrInitNono(uid int64) (*Nono, error) {
	n, err := s.GetNono(uid)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return DefaultNono(uid), nil
	}
	return n, nil
}

// UpsertNono 写入/更新。
func (s *sqlBackend) UpsertNono(n *Nono) error {
	if n == nil {
		return nil
	}
	if n.Nick == "" {
		n.Nick = "NoNo"
	}
	// Color=0 合法（钢琴黑变色芯片），勿改成默认白。
	if n.Power <= 0 {
		n.Power = NonoDefaultPower
	}
	if n.Mate <= 0 {
		n.Mate = NonoDefaultMate
	}
	if n.Birth <= 0 {
		n.Birth = time.Now().Unix()
	}
	bits := normalizeFuncBits(n.FuncBits)
	charged := 0
	if n.OpenGoldCharged {
		charged = 1
	}
	_, err := s.db.Exec(`
INSERT INTO user_nono (
  user_id, has_nono, flag, state, nick, color, super_nono, power, mate, iq, ai,
  birth, charge_time, func_bits, super_energy, super_level, super_stage, super_months,
  vip_value, auto_charge, vip_end_time, open_gold_charged
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
  has_nono=VALUES(has_nono), flag=VALUES(flag), state=VALUES(state), nick=VALUES(nick),
  color=VALUES(color), super_nono=VALUES(super_nono), power=VALUES(power), mate=VALUES(mate),
  iq=VALUES(iq), ai=VALUES(ai), birth=VALUES(birth), charge_time=VALUES(charge_time),
  func_bits=VALUES(func_bits), super_energy=VALUES(super_energy), super_level=VALUES(super_level),
  super_stage=VALUES(super_stage), super_months=VALUES(super_months), vip_value=VALUES(vip_value),
  auto_charge=VALUES(auto_charge), vip_end_time=VALUES(vip_end_time),
  open_gold_charged=VALUES(open_gold_charged)`,
		n.UserID, n.HasNono, n.Flag, n.State, n.Nick, n.Color, n.SuperNono,
		n.Power, n.Mate, n.IQ, n.AI, n.Birth, n.ChargeTime, bits,
		n.SuperEnergy, n.SuperLevel, n.SuperStage, n.SuperMonths,
		n.VipValue, n.AutoCharge, n.VipEndTime, charged)
	return err
}

func normalizeFuncBits(b []byte) []byte {
	out := make([]byte, 20)
	copy(out, b)
	return out
}

// IsFollowing 是否跟随中。
func (n *Nono) IsFollowing() bool {
	return n != nil && (n.State&NonoStateFollowBit) != 0
}

// SetFollowing 设置跟随位。
func (n *Nono) SetFollowing(on bool) {
	if n == nil {
		return
	}
	if on {
		n.State |= NonoStateFollowBit
	} else {
		n.State &^= NonoStateFollowBit
	}
}
