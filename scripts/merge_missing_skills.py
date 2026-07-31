# -*- coding: utf-8 -*-
"""Merge pet-referenced skill IDs missing from SkillXMLInfo from reference skills.xml."""
from __future__ import print_function

import re
import shutil
from pathlib import Path

ROOT = Path(r"d:\kaiyuanhao\niaohao")
REF = ROOT / "参考原代码，只供参加功能实现逻辑" / "data" / "skills.xml"
DST_XML = ROOT / "server" / "tables" / "xml" / "SkillXMLInfo.xml"
DST_BIN = ROOT / "server" / "tables" / "bin" / "SkillXMLInfo.bin"
DST_ASSET = ROOT / "反编译导出文件" / "nieocore" / "scripts" / "_assets" / "36_com.robot.core.config.xml.SkillXMLInfo_xmlClass.bin"
PET_FE = ROOT / "server" / "tables" / "bin" / "PetXMLInfo.bin"


def main():
    print("reading...")
    pets = PET_FE.read_bytes().decode("utf-8", "replace")
    cur = DST_XML.read_bytes().decode("utf-8", "replace")
    ref = REF.read_bytes().decode("utf-8", "replace")
    print("parsed sizes", len(pets), len(cur), len(ref))

    cur_ids = set(int(x) for x in re.findall(r'ID="(\d+)"', cur) if True)
    # tighter: only Move ID in skill file
    cur_ids = set(int(x) for x in re.findall(r'<Move[^>]*\bID="(\d+)"', cur))
    pet_moves = set(int(x) for x in re.findall(r'<Move\s+ID="(\d+)"', pets))
    missing = [mid for mid in sorted(pet_moves) if mid not in cur_ids]
    print("missing", len(missing))

    # index ref by scanning self-closing Moves only (ref skills.xml style)
    ref_map = {}
    for m in re.finditer(r"<Move\b[^>]*?\bID=\"(\d+)\"[^>]*?/>", ref):
        ref_map[int(m.group(1))] = m.group(0)
    print("ref moves", len(ref_map))

    inserts = []
    missing_ref = []
    for mid in missing:
        block = ref_map.get(mid)
        if not block:
            missing_ref.append(mid)
            continue
        inserts.append(block.strip())
    print("to insert", len(inserts), "no-ref", len(missing_ref))

    if not inserts:
        print("nothing to insert")
        return

    if "</Moves>" in cur:
        anchor = "</Moves>"
    elif "</root>" in cur:
        anchor = "</root>"
    else:
        raise SystemExit("unknown SkillXMLInfo root")

    chunk = "\n".join("        " + x for x in inserts) + "\n"
    new_cur = cur.replace(anchor, chunk + anchor, 1)

    bak = DST_XML.with_suffix(".xml.bak_pre_merge")
    if not bak.exists():
        shutil.copy2(str(DST_XML), str(bak))
        print("backup", bak)

    print("writing xml...")
    DST_XML.write_bytes(new_cur.encode("utf-8"))
    print("writing bin...")
    DST_BIN.write_bytes(new_cur.encode("utf-8"))
    if DST_ASSET.exists():
        shutil.copy2(str(DST_BIN), str(DST_ASSET))
        print("copied asset")

    new_ids = set(int(x) for x in re.findall(r'<Move[^>]*\bID="(\d+)"', new_cur))
    still = [m for m in missing if m not in new_ids]
    print("skill_total=%d still_missing=%d" % (len(new_ids), len(still)))
    for mid in (21424, 21425, 21426):
        print(mid, "present", mid in new_ids)


if __name__ == "__main__":
    main()
