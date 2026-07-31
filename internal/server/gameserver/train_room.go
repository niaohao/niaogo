package gameserver

import (
	"fmt"
	"log"
)

// 学习力学院：初级 488–493 每场 +1；高级 470–475 每场 +9（对应 HP/Atk/Def/SA/SD/Spd）。
var trainRoomMonLists = map[int][]int{
	488: {10, 13, 33, 126, 164},
	489: {228, 330, 228, 330, 228},
	490: {19, 62, 108, 158, 184},
	491: {232, 328, 232, 328, 232},
	492: {22, 25, 43, 153, 319},
	493: {56, 105, 456, 482, 111},
	470: {12, 15, 29, 55, 73},
	471: {202, 149, 213, 331, 231},
	472: {21, 37, 64, 79, 130},
	473: {234, 32, 197, 248, 290},
	474: {24, 45, 155, 321, 321},
	475: {58, 107, 458, 58, 458},
}

func trainRoomStatIndex(mapID int) (statIdx, amount int, ok bool) {
	switch mapID {
	case 488:
		return 0, 1, true
	case 489:
		return 1, 1, true
	case 490:
		return 2, 1, true
	case 491:
		return 3, 1, true
	case 492:
		return 4, 1, true
	case 493:
		return 5, 1, true
	case 470:
		return 0, 9, true
	case 471:
		return 1, 9, true
	case 472:
		return 2, 9, true
	case 473:
		return 3, 9, true
	case 474:
		return 4, 9, true
	case 475:
		return 5, 9, true
	}
	return 0, 0, false
}

func isTrainRoomMap(mapID int) bool {
	_, _, ok := trainRoomStatIndex(mapID)
	return ok
}

func resolveTrainRoomEnemy(mapID int, param2 uint32) (petID, level int, name string, ok bool) {
	list := trainRoomMonLists[mapID]
	if len(list) == 0 {
		return 0, 0, "", false
	}
	idx := int(param2)
	if idx < 0 || idx >= len(list) {
		idx = 0
	}
	petID = list[idx]
	level = 25
	if mapID >= 470 && mapID <= 475 {
		level = 50
	}
	name = fmt.Sprintf("训练精灵%d", petID)
	return petID, level, name, true
}

func trainRoomEVYield(mapID int) (yield [6]int, ok bool) {
	idx, amt, ok := trainRoomStatIndex(mapID)
	if !ok {
		return yield, false
	}
	yield[idx] = amt
	return yield, true
}

func logTrainEV(uid int64, mapID int, yield [6]int) {
	log.Printf("[train] EV UID=%d map=%d +%v", uid, mapID, yield)
}
