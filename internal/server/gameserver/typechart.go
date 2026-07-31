package gameserver

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// 属性克制：优先读 data/type_chart.json + type_components.json；失败则用内置简化表。

type typeChartRuntime struct {
	mu         sync.RWMutex
	cache      map[int]map[int]float64
	components map[int][]int
	loaded     bool
}

var battleTypes typeChartRuntime

func loadTypeChart(chartPath, componentsPath string) error {
	data, err := os.ReadFile(chartPath)
	if err != nil {
		return err
	}
	var parsed struct {
		Chart map[string]map[string]float64 `json:"chart"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	cache := make(map[int]map[int]float64, len(parsed.Chart))
	for atkStr, inner := range parsed.Chart {
		atkID, e := strconv.Atoi(atkStr)
		if e != nil || atkID <= 0 {
			continue
		}
		row := make(map[int]float64, len(inner))
		for defStr, mult := range inner {
			defID, e := strconv.Atoi(defStr)
			if e != nil || defID <= 0 {
				continue
			}
			row[defID] = mult
		}
		cache[atkID] = row
	}

	compData, err := os.ReadFile(componentsPath)
	if err != nil {
		return err
	}
	var compParsed struct {
		Components map[string][]int `json:"components"`
	}
	if err := json.Unmarshal(compData, &compParsed); err != nil {
		return err
	}
	components := make(map[int][]int, len(compParsed.Components))
	for idStr, comps := range compParsed.Components {
		id, e := strconv.Atoi(idStr)
		if e != nil || id <= 0 || len(comps) == 0 {
			continue
		}
		components[id] = comps
	}

	battleTypes.mu.Lock()
	battleTypes.cache = cache
	battleTypes.components = components
	battleTypes.loaded = true
	battleTypes.mu.Unlock()
	log.Printf("[typechart] loaded chart=%d atk-rows components=%d from %s",
		len(cache), len(components), chartPath)
	return nil
}

func (c *typeChartRuntime) chartMult(atkType, defType int) float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loaded {
		return -1 // 哨兵：未加载
	}
	if row, ok := c.cache[atkType]; ok {
		if v, ok := row[defType]; ok {
			return v
		}
	}
	return 1
}

func (c *typeChartRuntime) compsOf(typeID int) []int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loaded || typeID <= 0 {
		return nil
	}
	comps := c.components[typeID]
	if len(comps) == 0 {
		return nil
	}
	out := make([]int, len(comps))
	copy(out, comps)
	return out
}

func typeMultiplier(atkType, defType int) float64 {
	if atkType <= 0 || defType <= 0 {
		return 1
	}
	if battleTypes.loaded {
		// 直接查矩阵；复合防守属性无列时按成分连乘（对齐参考 Dual）
		battleTypes.mu.RLock()
		row, hasRow := battleTypes.cache[atkType]
		direct, hasDirect := float64(1), false
		if hasRow {
			if v, ok := row[defType]; ok {
				direct, hasDirect = v, true
			}
		}
		comps := battleTypes.components[defType]
		battleTypes.mu.RUnlock()
		if hasDirect {
			return direct
		}
		if len(comps) >= 2 {
			m := 1.0
			for _, c := range comps {
				m *= battleTypes.chartMult(atkType, c)
			}
			return m
		}
		return 1
	}
	key := atkType<<8 | defType
	if m, ok := typeChartSimple[key]; ok {
		return m
	}
	return 1
}

// stabBonus 本系技能 1.5 倍（复合属性看成分）。
func stabBonus(skillType, petType int) float64 {
	if skillType <= 0 || petType <= 0 {
		return 1
	}
	if skillType == petType {
		return 1.5
	}
	for _, c := range battleTypes.compsOf(petType) {
		if skillType == c {
			return 1.5
		}
	}
	return 1
}

// LoadBattleData 加载对战静态数据（属性表 + 野怪图池）。dir 通常为 server/data。
func LoadBattleData(dir string) error {
	if dir == "" {
		return fmt.Errorf("empty data dir")
	}
	chartErr := loadTypeChart(
		filepath.Join(dir, "type_chart.json"),
		filepath.Join(dir, "type_components.json"),
	)
	wildErr := loadMapWildConfig(filepath.Join(dir, "map_wild_config.json"))
	if chartErr != nil {
		log.Printf("[typechart] fallback simple: %v", chartErr)
	}
	if wildErr != nil {
		log.Printf("[ogre] map_wild_config missing: %v", wildErr)
	}
	if chartErr != nil && wildErr != nil {
		return fmt.Errorf("typechart: %v; wild: %v", chartErr, wildErr)
	}
	return nil
}

var typeChartSimple = map[int]float64{
	3<<8 | 1: 2, 3<<8 | 2: 0.5, 3<<8 | 9: 2, 3<<8 | 6: 2,
	2<<8 | 3: 2, 2<<8 | 1: 0.5, 2<<8 | 7: 2,
	1<<8 | 2: 2, 1<<8 | 3: 0.5, 1<<8 | 7: 2, 1<<8 | 4: 0.5,
	5<<8 | 2: 2, 5<<8 | 4: 2, 5<<8 | 7: 0, 5<<8 | 1: 0.5,
	4<<8 | 1: 2, 4<<8 | 5: 0.5, 4<<8 | 11: 2,
	7<<8 | 3: 2, 7<<8 | 5: 2, 7<<8 | 6: 2, 7<<8 | 4: 0, 7<<8 | 1: 0.5,
	9<<8 | 1: 2, 9<<8 | 4: 2, 9<<8 | 7: 2, 9<<8 | 3: 0.5, 9<<8 | 2: 0.5,
	11<<8 | 8: 2, 11<<8 | 9: 2, 11<<8 | 4: 0.5,
	8<<8 | 6: 0.5,
	6<<8 | 9: 2, 6<<8 | 3: 0.5, 6<<8 | 2: 0.5, 6<<8 | 5: 0.5,
}
