package tableloader

import (
	"encoding/xml"
	"fmt"
	"os"
	"sync"
)

// EvolveBranch 与前端 EvolveXMLInfo.getMonToIDs 一致。
type EvolveBranch struct {
	MonTo          int
	EvolvItem      int
	EvolvItemCount int
}

type evolveBranchXML struct {
	MonTo          int `xml:"MonTo,attr"`
	EvolvItem      int `xml:"EvolvItem,attr"`
	EvolvItemCount int `xml:"EvolvItemCount,attr"`
}

type evolveNodeXML struct {
	ID       int               `xml:"ID,attr"`
	Branches []evolveBranchXML `xml:"Branch"`
}

type evolveRootXML struct {
	Evolves []evolveNodeXML `xml:"Evolve"`
}

var (
	evolveOnce sync.Once
	evolveErr  error
	evolveByFlag map[int][]EvolveBranch
)

// LoadEvolveXML 加载 EvolveXMLInfo.xml（可重复调用，仅首次生效）。
func LoadEvolveXML(path string) error {
	evolveOnce.Do(func() {
		evolveByFlag = make(map[int][]EvolveBranch)
		b, err := os.ReadFile(path)
		if err != nil {
			evolveErr = err
			return
		}
		var root evolveRootXML
		if err := xml.Unmarshal(b, &root); err != nil {
			evolveErr = err
			return
		}
		for _, e := range root.Evolves {
			if e.ID <= 0 {
				continue
			}
			list := make([]EvolveBranch, 0, len(e.Branches))
			for _, br := range e.Branches {
				if br.MonTo <= 0 {
					continue
				}
				list = append(list, EvolveBranch{
					MonTo: br.MonTo, EvolvItem: br.EvolvItem, EvolvItemCount: br.EvolvItemCount,
				})
			}
			if len(list) > 0 {
				evolveByFlag[e.ID] = list
			}
		}
		fmt.Printf("[tables] evolve flags=%d path=%s\n", len(evolveByFlag), path)
	})
	return evolveErr
}

// EvolveBranches 按 EvolvFlag 取分支（1-based index 由调用方处理）。
func EvolveBranches(flag int) ([]EvolveBranch, bool) {
	if evolveByFlag == nil {
		return nil, false
	}
	b, ok := evolveByFlag[flag]
	return b, ok && len(b) > 0
}
