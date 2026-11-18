#!/usr/bin/env python3
"""Merge a coverage file written by the end-to-end driver into coverage.tsv.

    E2E_COVERAGE=run.tsv go test -tags e2e ./test/e2e/ ...
    test/inventory/record.py run.tsv

The driver writes one row per item and case that passed: kind, item,
"class:case". This adds each token to the third column of the matching row of
coverage.tsv. A row the inventory does not know is reported and skipped, so a
typo in a case cannot invent coverage.
"""
import os
import sys

here = os.path.dirname(os.path.abspath(__file__))
path = os.path.join(here, "coverage.tsv")

rows, order = {}, []
with open(path) as f:
    for line in f:
        parts = line.rstrip("\n").split("\t")
        if len(parts) < 2:
            continue
        key = (parts[0], parts[1])
        rows[key] = set(parts[2].split()) if len(parts) > 2 and parts[2] else set()
        order.append(key)

added, unknown = 0, []
for src in sys.argv[1:]:
    with open(src) as f:
        for line in f:
            parts = line.rstrip("\n").split("\t")
            if len(parts) != 3:
                continue
            key = (parts[0], parts[1])
            if key not in rows:
                unknown.append("%s\t%s" % key)
                continue
            if parts[2] not in rows[key]:
                rows[key].add(parts[2])
                added += 1


def sort_key(token):
    cls, _, case = token.partition(":")
    num = int(cls[1:]) if cls[:1] == "E" and cls[1:].isdigit() else 0
    return (num, case)


with open(path, "w") as f:
    for key in order:
        f.write("%s\t%s\t%s\n" % (key[0], key[1], " ".join(sorted(rows[key], key=sort_key))))

print("%d tokens added to %s" % (added, os.path.relpath(path)))
for u in sorted(set(unknown)):
    print("NOT IN INVENTORY\t" + u)
sys.exit(1 if unknown else 0)
