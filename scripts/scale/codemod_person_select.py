#!/usr/bin/env python3
"""Rewrites `<Select>` lists built from a full people array into PersonSelect /
PeopleSelect (server-searched, see web/app/src/components/PeoplePicker.tsx).

Only the exact shape  <Select ...props>[<Option value="">LABEL</Option>]{LIST.map((p) => <Option key={p.id} value={p.id}>{p.name}</Option>)}</Select>
is rewritten, where LIST looks like a people array. Anything else is left alone.
Usage: codemod_person_select.py FILE...   (prints what it changed)
"""
import re
import sys

PEOPLE = r"(?:ctx\.people|data\.users|people|users|members|staff|employees|team|orgMembers|dir|persons|assignees|owners)"
MAP = re.compile(
    r"(?:<Option\s+value=\"\"[^>]*>(?P<none>[^<]*)</Option>\s*)?"
    r"\{\s*(?P<list>" + PEOPLE + r"(?:\.filter\((?:[^()]|\([^()]*\))*\))?)\s*\.map\(\s*\((?P<v>\w+)\)\s*=>\s*"
    r"\(?\s*<Option\s+key=\{(?P=v)\.id\}\s+value=\{(?P=v)\.id\}\s*>\s*\{(?P=v)\.name(?:\s*\|\|\s*(?P=v)\.email)?\}\s*</Option>\s*\)?\s*\)\s*\}\s*$",
    re.S,
)


def match_brace(s, i):
    """s[i] == '{' -> index of the matching '}'."""
    depth = 0
    j = i
    while j < len(s):
        c = s[j]
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return j
        j += 1
    return -1


def prop(tag, name):
    m = re.search(r"\b" + name + r"=\{", tag)
    if not m:
        return None
    start = m.end() - 1
    end = match_brace(tag, start)
    return tag[start + 1 : end], m.start(), end + 1


def convert(src):
    out = []
    pos = 0
    n = 0
    for m in re.finditer(r"<Select\b", src):
        if m.start() < pos:
            continue
        # end of opening tag: first '>' not inside braces
        i = m.end()
        depth = 0
        while i < len(src):
            c = src[i]
            if c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
            elif c == ">" and depth == 0 and src[i - 1] != "=":
                break
            i += 1
        else:
            continue
        tag = src[m.start() : i + 1]
        if tag.endswith("/>"):
            continue
        close = src.find("</Select>", i)
        if close < 0:
            continue
        inner = src[i + 1 : close]
        if "<Select" in inner:
            continue
        mm = MAP.search(inner)
        if not mm or inner[: mm.start()].strip():
            continue
        v = prop(tag, "value")
        oc = prop(tag, "onChange")
        if not v or not oc:
            continue
        multiple = re.search(r"\bmultiple\b", tag) is not None
        body = oc[0].strip()
        am = re.match(r"\(\s*_\s*,\s*(\w+)\s*\)\s*=>\s*(.*)$", body, re.S)
        if not am:
            continue
        arg, expr = am.group(1), am.group(2)
        param = "ids" if multiple else "id"
        expr = re.sub(r"\b" + arg + r"\s*\?\?\s*(?:\"\"|null|undefined)", param, expr)
        expr = re.sub(r"\b" + arg + r"\b(?!\.)", param, expr)
        if re.search(r"\b" + arg + r"\b", expr):
            continue
        none = (mm.group("none") or "").strip()
        ph = re.search(r"\bplaceholder=(\"[^\"]*\"|\{[^}]*\})", tag)
        placeholder = ph.group(1) if ph else (f'"{none}"' if none else None)
        comp = "PeopleSelect" if multiple else "PersonSelect"
        attrs = [f"value={{{v[0]}}}", f"known={{{mm.group('list')}}}", f"onChange={{({param}) => {expr}}}"]
        if placeholder:
            attrs.append(f"placeholder={placeholder}")
        al = re.search(r"aria-label=(\"[^\"]*\")", tag)
        attrs.append(f"ariaLabel={al.group(1)}" if al else 'ariaLabel="Person"')
        td = re.search(r'"data-testid":\s*([^,}]+)', tag)
        if td:
            attrs.append(f"testId={{{td.group(1).strip()}}}")
        new = f"<{comp} " + " ".join(attrs) + " />"
        out.append(src[pos : m.start()])
        out.append(new)
        pos = close + len("</Select>")
        n += 1
    out.append(src[pos:])
    return "".join(out), n


def main():
    total = 0
    for p in sys.argv[1:]:
        s = open(p).read()
        new, n = convert(s)
        if n:
            if "components/PeoplePicker" not in new:
                depth = p.split("/src/")[1].count("/")
                rel = "../" * depth
                new = new.replace("import ", f'import {{ PersonSelect, PeopleSelect }} from "{rel}components/PeoplePicker";\nimport ', 1)
            open(p, "w").write(new)
            print(f"{n:3d}  {p}")
            total += n
    print("total", total)


main()
