# -*- coding: utf-8 -*-
"""从参考 mapogres.go 的 maps 表生成 map_wild_config.json（本服 petId 格式）。"""
from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PETS_XML = ROOT / "server" / "tables" / "xml" / "pets.xml"
REF_GO = ROOT / "参考原代码，只供参加功能实现逻辑" / "internal" / "game" / "mapogres" / "mapogres.go"
OUT_JSON = ROOT / "server" / "data" / "map_wild_config.json"


def load_pets():
    text = PETS_XML.read_text(encoding="utf-8")
    name2id: dict[str, int] = {}
    evolves_from: dict[int, int] = {}
    for m in re.finditer(r"<Monster\b([^>]*)/?>", text):
        attrs = m.group(1)

        def get(a: str) -> str:
            mm = re.search(rf'{a}="([^"]*)"', attrs)
            return mm.group(1) if mm else ""

        # 跳过 RealId 别名行（如 1400105 格林）
        if get("RealId"):
            continue
        pid_s = get("ID")
        name = get("DefName") or get("Name")
        if not pid_s or not name:
            continue
        pid = int(pid_s)
        # 同名保留最小 ID（主形态）
        if name not in name2id or pid < name2id[name]:
            name2id[name] = pid
        ef = get("EvolvesFrom")
        evolves_from[pid] = int(ef) if ef.isdigit() else 0
    return name2id, evolves_from


def load_fallback(ref: str) -> dict[str, int]:
    start = ref.find("var nameToIDFallback = map[string]int{")
    if start < 0:
        raise SystemExit("nameToIDFallback not found")
    i = ref.find("{", start)
    depth = 0
    j = i
    while j < len(ref):
        if ref[j] == "{":
            depth += 1
        elif ref[j] == "}":
            depth -= 1
            if depth == 0:
                j += 1
                break
        j += 1
    block = ref[start:j]
    fb: dict[str, int] = {}
    for m in re.finditer(r'"([^"]+)":\s*(\d+)', block):
        fb[m.group(1)] = int(m.group(2))
    # 覆盖同名高 ID 形态，强制经典主 ID
    overrides = {
        "雷伊": 70,
        "格林": 62,
        "达尔": 122,
        "晶岩兽": 203,
        "火炎贝": 38,
        "西塔": 59,
    }
    fb.update(overrides)
    return fb


def resolve_name(name: str, name2id: dict[str, int], fb: dict[str, int]) -> int:
    n = name.lstrip("#").strip()
    if n in fb:
        return fb[n]
    if n in name2id:
        return name2id[n]
    if n.startswith("闪光"):
        if n in fb:
            return fb[n]
    return 0


def extract_map_bodies(ref: str) -> dict[int, str]:
    maps_start = ref.find("var maps = map[int]mapConfig{")
    if maps_start < 0:
        raise SystemExit("maps not found")
    # 地图表结束于 planetFallback 声明之前
    m_end = ref.find("\nvar planetFallback", maps_start)
    if m_end < 0:
        m_end = ref.find("\r\nvar planetFallback", maps_start)
    if m_end < 0:
        # 兼容注释行：// planetFallback
        m_end = ref.find("planetFallback", maps_start)
        m_end = ref.rfind("\n", maps_start, m_end)
    maps_block = ref[maps_start:m_end]
    entries: dict[int, str] = {}
    for m in re.finditer(r"\n\t(\d+):\s*\{", maps_block):
        map_id = int(m.group(1))
        i = m.end() - 1
        depth = 0
        j = i
        while j < len(maps_block):
            c = maps_block[j]
            if c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    j += 1
                    break
            j += 1
        entries[map_id] = maps_block[i:j]
    return entries


def parse_field(body: str, field: str) -> list[dict]:
    # 必须匹配配置字段「Common:」「Rare:」「Monsters:」，避免命中条目内 Rare: true/false
    if field.startswith("Common"):
        m = re.search(r"\bCommon:\s*\[", body)
    elif field.startswith("Rare"):
        m = re.search(r"\bRare:\s*\[", body)
    elif field.startswith("Monsters"):
        m = re.search(r"\bMonsters:\s*\[", body)
    else:
        return []
    if not m:
        return []
    idx = m.start()
    brace = body.find("{", idx)
    if brace < 0:
        return []
    depth = 0
    k = brace
    while k < len(body):
        ch = body[k]
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                k += 1
                break
        k += 1
    arr = body[brace:k]
    res: list[dict] = []
    if field.startswith("Monsters"):
        for sm in re.finditer(r'"([^"]+)"', arr):
            res.append({"name": sm.group(1), "levelMin": 5, "levelMax": 5})
        return res
    for sm in re.finditer(r"\{([^{}]*)\}", arr):
        inner = sm.group(1)
        nm = re.search(r'Name:\s*"([^"]+)"', inner)
        if not nm:
            continue
        lmin = re.search(r"LevelMin:\s*(\d+)", inner)
        lmax = re.search(r"LevelMax:\s*(\d+)", inner)
        res.append(
            {
                "name": nm.group(1),
                "levelMin": int(lmin.group(1)) if lmin else 0,
                "levelMax": int(lmax.group(1)) if lmax else 0,
            }
        )
    return res


def main():
    name2id, evolves_from = load_pets()
    ref = REF_GO.read_text(encoding="utf-8")
    fb = load_fallback(ref)
    entries = extract_map_bodies(ref)
    unresolved: list[tuple[int, str]] = []
    out_maps: dict[str, dict] = {}

    for mid, body in sorted(entries.items()):
        common = parse_field(body, "Common:")
        rare = parse_field(body, "Rare:")
        monsters = parse_field(body, "Monsters:")
        if not common and not rare and monsters:
            common = monsters

        def to_list(lst: list[dict]) -> list[dict]:
            out = []
            for e in lst:
                pid = resolve_name(e["name"], name2id, fb)
                if pid <= 0:
                    unresolved.append((mid, e["name"]))
                    continue
                lmin, lmax = e["levelMin"], e["levelMax"]
                if lmin <= 0 and lmax <= 0:
                    lmin, lmax = 5, 5
                if lmax < lmin:
                    lmax = lmin
                cc = evolves_from.get(pid, 0) <= 0
                out.append(
                    {
                        "petId": pid,
                        "levelMin": lmin,
                        "levelMax": lmax,
                        "canCatch": cc,
                    }
                )
            return out

        c = to_list(common)
        r = to_list(rare)
        if not c and not r:
            continue
        out_maps[str(mid)] = {"common": c, "rare": r}

    doc = {
        "shinyRate": 0,
        "rareEmptySlots": 20,
        "comment": "由 scripts/gen_map_wild_config.py 从参考 mapogres.go 生成；canCatch=false 当 EvolvesFrom>0；shinyRate=0 因本客户端 2004 无异色字段",
        "maps": out_maps,
    }
    OUT_JSON.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    uniq = sorted({n for _, n in unresolved})
    print(f"wrote {OUT_JSON} maps={len(out_maps)} unresolved={len(uniq)}")
    if uniq:
        print("missing names:")
        for n in uniq:
            print(" ", n)


if __name__ == "__main__":
    main()
