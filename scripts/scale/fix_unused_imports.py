#!/usr/bin/env python3
"""Removes names reported as unused (TS6133) from import lists.
Usage: fix_unused_imports.py <tsc-log> <web/app dir>"""
import re
import sys

log, root = sys.argv[1], sys.argv[2]
hits = {}
for line in open(log):
    m = re.match(r"(src/[^(]+)\((\d+),\d+\): error TS61(?:33|96): '(\w+)' is declared", line)
    if m:
        hits.setdefault(m.group(1), []).append((int(m.group(2)), m.group(3)))
for path, items in hits.items():
    p = f"{root}/{path}"
    lines = open(p).read().split("\n")
    for ln, name in sorted(items, reverse=True):
        i = ln - 1
        # find the import statement containing this line
        start = i
        while start > 0 and not lines[start].lstrip().startswith("import"):
            start -= 1
        end = i
        while end < len(lines) - 1 and not re.search(r"from\s+\"[^\"]+\";?\s*$", lines[end]):
            end += 1
        stmt = "\n".join(lines[start : end + 1])
        if not stmt.lstrip().startswith("import"):
            print("skip (not an import):", path, ln, name)
            continue
        new = re.sub(r"(\{[^}]*)\b(?:type\s+)?" + name + r"\b\s*,?\s*", r"\1", stmt, count=1)
        new = re.sub(r",\s*\}", " }", new)
        new = re.sub(r"\{\s*,", "{ ", new)
        if re.search(r"import\s*\{\s*\}\s*from", new):
            new = ""
        lines[start : end + 1] = [new] if new else []
        print("fixed", path, name)
    open(p, "w").write("\n".join(lines))
