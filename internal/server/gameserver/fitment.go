package gameserver

import (
	"sort"

	"niaohao/server/internal/store"
)

const (
	maxPlacedFitments    = 120
	fitmentFrameMinID    = 500001
	fitmentFrameMaxID    = 500100
	fitmentItemMinID     = 500001
	fitmentItemMaxSanity = 599999
)

// 功能家具：进基地发仓库，并自动摆到默认坐标（对齐参考服，避免仓库有货但场景点不到）。
// 500502 精灵恢复仓 Fun=2 → Ext_2.swf → PetManager.cureAll → 2306
// 500501 精灵仓库 Fun=1 → Ext_1 → PetStorage（依赖 2303 + 2324）
// 500503 分子转化仪 Fun=3 → Ext_3 → App_700002 → 2315/2316
// 500514 经验分配器 Fun=5 → 2318/2319
// 500655 精灵培育仓 → 2364–2370/2374
var starterFitmentWarehouseGrant = []int{
	500001, // 默认房型
	500502, // 精灵恢复仓
	500503, // 分子转化仪
	500514, // 经验分配器
	500501, // 精灵仓库
	500655, // 精灵培育仓
}

// 已有基地档案时补回的仓库家具（参考服仅补这两件）。
var starterFitmentWarehouseIDs = []int{500502, 500503}

// 仓库有货但未摆放时自动摆放（坐标贴近官服默认布局）。
var starterFitmentAutoPlace = []struct {
	id int
	x  int
	y  int
}{
	{500502, 380, 260}, // 精灵恢复仓
	{500503, 520, 260}, // 分子转化仪
	{500514, 450, 200}, // 经验分配器
	{500501, 260, 260}, // 精灵仓库
	{500655, 600, 280}, // 精灵培育仓
}

// fitmentBag 内存态：仓库数量 + 已摆放列表。
type fitmentBag struct {
	Fitments []store.Fitment
	Items    map[int]int // itemID -> warehouse count
}

func isValidFitmentID(id int) bool {
	if id < fitmentItemMinID || id > fitmentItemMaxSanity {
		return false
	}
	if id >= fitmentFrameMinID && id <= fitmentFrameMaxID {
		return true
	}
	return id >= 500101
}

func clampFitmentFields(f *store.Fitment) {
	if f == nil {
		return
	}
	if f.X < 0 || f.X > 2000 {
		f.X = 0
	}
	if f.Y < 0 || f.Y > 2000 {
		f.Y = 0
	}
	if f.Dir < 0 || f.Dir > 7 {
		f.Dir = 0
	}
	if f.Status < 0 {
		f.Status = 0
	}
}

func hasRoomFrame(fitments []store.Fitment) bool {
	for _, f := range fitments {
		if f.ID >= fitmentFrameMinID && f.ID <= fitmentFrameMaxID {
			return true
		}
	}
	return false
}

func fitmentItemCount(bag *fitmentBag, itemID int) int {
	if bag == nil || bag.Items == nil || itemID <= 0 {
		return 0
	}
	return bag.Items[itemID]
}

func fitmentPlacedCount(bag *fitmentBag, itemID int) int {
	if bag == nil || itemID <= 0 {
		return 0
	}
	n := 0
	for _, f := range bag.Fitments {
		if f.ID == itemID {
			n++
		}
	}
	return n
}

func userHasBaseHome(bag *fitmentBag) bool {
	if bag == nil {
		return false
	}
	if fitmentItemCount(bag, defaultRoomStyleID) > 0 || hasRoomFrame(bag.Fitments) {
		return true
	}
	for _, id := range starterFitmentWarehouseGrant {
		if id == defaultRoomStyleID {
			continue
		}
		if fitmentItemCount(bag, id) > 0 || fitmentPlacedCount(bag, id) > 0 {
			return true
		}
	}
	return false
}

func sanitizeUserFitments(bag *fitmentBag) (removed int) {
	if bag == nil {
		return 0
	}
	src := bag.Fitments
	if src == nil {
		bag.Fitments = []store.Fitment{}
		return 0
	}
	out := make([]store.Fitment, 0, len(src))
	frameIdx := -1
	for _, f := range src {
		if f.ID <= 0 || !isValidFitmentID(f.ID) {
			removed++
			continue
		}
		ff := f
		clampFitmentFields(&ff)
		if ff.ID >= fitmentFrameMinID && ff.ID <= fitmentFrameMaxID {
			if frameIdx >= 0 {
				out[frameIdx] = ff
				removed++
				continue
			}
			frameIdx = len(out)
		}
		out = append(out, ff)
		if len(out) >= maxPlacedFitments {
			break
		}
	}
	// 仅有摆件无房型时补默认房型，避免客户端加载基地卡死/极慢
	if len(out) > 0 && !hasRoomFrame(out) {
		out = append([]store.Fitment{{ID: defaultRoomStyleID}}, out...)
	}
	bag.Fitments = out
	return removed
}

func ensureStarterFitmentWarehouseItems(bag *fitmentBag) bool {
	if bag == nil || !userHasBaseHome(bag) {
		return false
	}
	if bag.Items == nil {
		bag.Items = map[int]int{}
	}
	changed := false
	for _, id := range starterFitmentWarehouseIDs {
		if fitmentItemCount(bag, id) > 0 || fitmentPlacedCount(bag, id) > 0 {
			continue
		}
		bag.Items[id] = 1
		changed = true
	}
	return changed
}

// bootstrapStarterFitments 首次进基地：功能件发到仓库，不自动摆放。
func bootstrapStarterFitments(bag *fitmentBag) bool {
	if bag == nil || userHasBaseHome(bag) {
		return false
	}
	if bag.Items == nil {
		bag.Items = map[int]int{}
	}
	for _, id := range starterFitmentWarehouseGrant {
		if bag.Items[id] <= 0 && fitmentPlacedCount(bag, id) <= 0 {
			bag.Items[id] = 1
		}
	}
	return true
}

func fitmentPlacedCountMap(fitments []store.Fitment) map[int]int {
	m := make(map[int]int)
	for _, f := range fitments {
		if f.ID > 0 {
			m[f.ID]++
		}
	}
	return m
}

func takeFitmentFromWarehouse(bag *fitmentBag, itemID, n int) {
	if bag == nil || bag.Items == nil || itemID <= 0 || n <= 0 {
		return
	}
	cur := bag.Items[itemID]
	if cur <= 0 {
		return
	}
	if cur <= n {
		delete(bag.Items, itemID)
		return
	}
	bag.Items[itemID] = cur - n
}

func returnFitmentToWarehouse(bag *fitmentBag, itemID, n int) {
	if bag == nil || itemID <= 0 || n <= 0 {
		return
	}
	if bag.Items == nil {
		bag.Items = map[int]int{}
	}
	bag.Items[itemID] += n
}

func reconcileFitmentWarehouse(bag *fitmentBag, before, after []store.Fitment) {
	if bag == nil {
		return
	}
	oldC := fitmentPlacedCountMap(before)
	newC := fitmentPlacedCountMap(after)
	seen := make(map[int]struct{}, len(oldC)+len(newC))
	for id := range oldC {
		seen[id] = struct{}{}
	}
	for id := range newC {
		seen[id] = struct{}{}
	}
	for id := range seen {
		delta := newC[id] - oldC[id]
		if delta > 0 {
			takeFitmentFromWarehouse(bag, id, delta)
		} else if delta < 0 {
			returnFitmentToWarehouse(bag, id, -delta)
		}
	}
}

// ensureStarterPlacedFitments 将仓库中已有但未摆放的功能家具自动放入场景。
func ensureStarterPlacedFitments(bag *fitmentBag) bool {
	if bag == nil {
		return false
	}
	if bag.Fitments == nil {
		bag.Fitments = []store.Fitment{}
	}
	changed := false
	if !hasRoomFrame(bag.Fitments) {
		bag.Fitments = append([]store.Fitment{{ID: defaultRoomStyleID}}, bag.Fitments...)
		if fitmentItemCount(bag, defaultRoomStyleID) > 0 {
			takeFitmentFromWarehouse(bag, defaultRoomStyleID, 1)
		}
		changed = true
	}
	for _, sp := range starterFitmentAutoPlace {
		if fitmentItemCount(bag, sp.id) <= 0 {
			continue
		}
		if fitmentPlacedCount(bag, sp.id) > 0 {
			continue
		}
		ff := store.Fitment{ID: sp.id, X: sp.x, Y: sp.y, Dir: 0, Status: 0}
		clampFitmentFields(&ff)
		bag.Fitments = append(bag.Fitments, ff)
		takeFitmentFromWarehouse(bag, sp.id, 1)
		changed = true
	}
	if changed {
		sanitizeUserFitments(bag)
	}
	return changed
}

// syncOwnerFitmentsOnEnter 进入自己基地：补仓库 + 自动摆功能件 + 消毒（含默认房型）。
func syncOwnerFitmentsOnEnter(bag *fitmentBag) bool {
	if bag == nil {
		return false
	}
	changed := bootstrapStarterFitments(bag)
	if ensureStarterFitmentWarehouseItems(bag) {
		changed = true
	}
	if ensureStarterPlacedFitments(bag) {
		changed = true
	}
	if sanitizeUserFitments(bag) > 0 {
		changed = true
	}
	return changed
}

func buildFitmentStorageRows(bag *fitmentBag) [][3]int {
	if bag == nil {
		return nil
	}
	used := fitmentPlacedCountMap(bag.Fitments)
	type row struct{ id, used, all int }
	rows := make([]row, 0)
	seen := map[int]bool{}
	ids := make([]int, 0, len(bag.Items))
	for id, n := range bag.Items {
		if n <= 0 || !isValidFitmentID(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		rows = append(rows, row{id, used[id], bag.Items[id]})
		seen[id] = true
	}
	placedIDs := make([]int, 0)
	for id := range used {
		if !seen[id] && isValidFitmentID(id) {
			placedIDs = append(placedIDs, id)
		}
	}
	sort.Ints(placedIDs)
	for _, id := range placedIDs {
		u := used[id]
		rows = append(rows, row{id, u, u})
	}
	out := make([][3]int, len(rows))
	for i, r := range rows {
		out[i] = [3]int{r.id, r.used, r.all}
	}
	return out
}
