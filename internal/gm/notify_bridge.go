package gm

// NotifyFuncs 把 gameserver 方法适配为 Notifier（跨包 OnlinePlayer 类型不同）。
type NotifyFuncs struct {
	PushItem     func(uid int64, itemID, count int)
	PushPet      func(uid int64, petID int, catchTime int64)
	RefreshPet   func(uid, catchTime int64) // 改宠后 2508+2301
	PushCurrency func(uid int64)
	ListOnline   func() []OnlinePlayer
	Kick         func(uid int64) bool
}

func (n NotifyFuncs) PushItemGain(uid int64, itemID, count int) {
	if n.PushItem != nil {
		n.PushItem(uid, itemID, count)
	}
}
func (n NotifyFuncs) PushPetGain(uid int64, petID int, catchTime int64) {
	if n.PushPet != nil {
		n.PushPet(uid, petID, catchTime)
	}
}
func (n NotifyFuncs) PushPetRefresh(uid, catchTime int64) {
	if n.RefreshPet != nil {
		n.RefreshPet(uid, catchTime)
	}
}
func (n NotifyFuncs) PushCurrencyBalance(uid int64) {
	if n.PushCurrency != nil {
		n.PushCurrency(uid)
	}
}
func (n NotifyFuncs) ListOnlinePlayers() []OnlinePlayer {
	if n.ListOnline != nil {
		return n.ListOnline()
	}
	return nil
}
func (n NotifyFuncs) KickUser(uid int64) bool {
	if n.Kick != nil {
		return n.Kick(uid)
	}
	return false
}
