package tableloader

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// NatureMods 性格对五维（攻/防/特攻/特防/速）的倍率；HP 不受性格影响。
type NatureMods struct {
	Atk, Def, SpAtk, SpDef, Spd float64
}

var (
	natureOnce sync.Once
	natureErr  error
	natureByID map[int]NatureMods
)

// 与 NatureXMLInfo.xml 一致的兜底表（加载失败时仍可用）。
var natureFallback = map[int]NatureMods{
	0:  {1.1, 0.9, 1, 1, 1},
	1:  {1.1, 1, 0.9, 1, 1},
	2:  {1.1, 1, 1, 0.9, 1},
	3:  {1.1, 1, 1, 1, 0.9},
	4:  {0.9, 1.1, 1, 1, 1},
	5:  {1, 1.1, 0.9, 1, 1},
	6:  {1, 1.1, 1, 0.9, 1},
	7:  {1, 1.1, 1, 1, 0.9},
	8:  {0.9, 1, 1.1, 1, 1},
	9:  {1, 0.9, 1.1, 1, 1},
	10: {1, 1, 1.1, 0.9, 1},
	11: {1, 1, 1.1, 1, 0.9},
	12: {0.9, 1, 1, 1.1, 1},
	13: {1, 0.9, 1, 1.1, 1},
	14: {1, 1, 0.9, 1.1, 1},
	15: {1, 1, 1, 1.1, 0.9},
	16: {0.9, 1, 1, 1, 1.1},
	17: {1, 0.9, 1, 1, 1.1},
	18: {1, 1, 0.9, 1, 1.1},
	19: {1, 1, 1, 0.9, 1.1},
	20: {1, 1, 1, 1, 1},
	21: {1, 1, 1, 1, 1},
	22: {1, 1, 1, 1, 1},
	23: {1, 1, 1, 1, 1},
	24: {1, 1, 1, 1, 1},
}

type natureRootXML struct {
	Items []struct {
		ID       string `xml:"id,attr"`
		Attack   string `xml:"m_attack,attr"`
		Defence  string `xml:"m_defence,attr"`
		SpAtk    string `xml:"m_SA,attr"`
		SpDef    string `xml:"m_SD,attr"`
		Speed    string `xml:"m_speed,attr"`
	} `xml:"item"`
}

// LoadNatureXML 加载 NatureXMLInfo.xml（仅首次生效）。
func LoadNatureXML(path string) error {
	natureOnce.Do(func() {
		natureByID = make(map[int]NatureMods, 25)
		b, err := os.ReadFile(path)
		if err != nil {
			natureErr = fmt.Errorf("read nature: %w", err)
			return
		}
		var root natureRootXML
		if err := xml.Unmarshal(b, &root); err != nil {
			natureErr = fmt.Errorf("parse nature: %w", err)
			return
		}
		for _, it := range root.Items {
			id, err := strconv.Atoi(it.ID)
			if err != nil {
				continue
			}
			natureByID[id] = NatureMods{
				Atk:   parseNatureMul(it.Attack),
				Def:   parseNatureMul(it.Defence),
				SpAtk: parseNatureMul(it.SpAtk),
				SpDef: parseNatureMul(it.SpDef),
				Spd:   parseNatureMul(it.Speed),
			}
		}
		if len(natureByID) == 0 {
			natureErr = fmt.Errorf("nature empty")
		}
	})
	return natureErr
}

func parseNatureMul(s string) float64 {
	if s == "" {
		return 1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return 1
	}
	return v
}

// NatureModsOf 取性格倍率；未知 ID 返回全 1。
func NatureModsOf(natureID int) NatureMods {
	if natureByID != nil {
		if m, ok := natureByID[natureID]; ok {
			return m
		}
	}
	if m, ok := natureFallback[natureID]; ok {
		return m
	}
	return NatureMods{1, 1, 1, 1, 1}
}
