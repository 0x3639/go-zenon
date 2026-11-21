#!/usr/bin/env python3
"""M-3: mainnet history scanner for the go-zenon test framework (level L6).

Two scans over mainnet history:

  --headers  Walk every momentum from 1 to a stated height in pages of 1000,
             verifying the hash links: each page must be height-contiguous
             and the first entry's previousHash must equal the hash of the
             last entry of the previous page.  Reports how many momentums
             were read and any link break.

  --blocks   Scan a stated height range reading full account blocks (10
             momentums per call), reporting how many momentums and account
             blocks were read, all ChangeTssECDSAPubKey sends (bridge key
             changes, method id 15a0c641 — the first 4 bytes of the SHA3-256
             of the method signature, computed below), and every momentum or
             block whose Difficulty read as unsigned 64-bit is >= 2^63.

Refuses to run unless the endpoint reports chain identifier 1 and the
mainnet genesis momentum hash at height 1.

Standard library Python 3 only.  Does not publish transactions.

Output: one JSON line per finding and per progress milestone, a final
summary, and the commit the tool was built from.

  python3 test/mainnet/scan.py --node http://127.0.0.1:35997 \
      --headers --to 14661812 --out scan.jsonl

  python3 test/mainnet/scan.py --node http://127.0.0.1:36997 \
      --blocks --from 1 --to 14661812 --out scan.jsonl
"""
import argparse
import base64
import hashlib
import json
import os
import subprocess
import sys
import time
import urllib.request

MAINNET_GENESIS_MOMENTUM = "9e204601d1b7b1427fe12bc82622e610d8a6ad43c40abf020eb66e538bb8eeb0"
MAINNET_CHAIN_ID = 1

# ChangeTssECDSAPubKey(string,string,string): first 4 bytes of SHA3-256,
# identical to vm/abi Method.Id() in go-zenon.
BRIDGE_KEY_SIG = "ChangeTssECDSAPubKey(string,string,string)"
BRIDGE_KEY_SELECTOR = hashlib.sha3_256(BRIDGE_KEY_SIG.encode()).digest()[:4]

HIGH_DIFF_THRESHOLD = 2**63
PAGE = 1000
DETAILED = 10           # getDetailedMomentumsByHeight count per call
PROGRESS_EVERY = 100_000


def log(line, fh):
    fh.write(json.dumps(line, default=str) + "\n")
    fh.flush()


def rpc(endpoint, method, params=None, timeout=120, retries=3):
    body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": method,
                       "params": params or []}).encode()
    last = None
    for attempt in range(retries):
        try:
            req = urllib.request.Request(endpoint, data=body,
                                         headers={"Content-Type": "application/json"})
            resp = urllib.request.urlopen(req, timeout=timeout)
            data = json.loads(resp.read())
            if "error" in data:
                raise RuntimeError(f"{method}: {data['error']}")
            return data["result"]
        except RuntimeError:
            raise
        except Exception as e:
            last = e
            time.sleep(2 * (attempt + 1))
    raise last


def tool_commit():
    try:
        root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
        out = subprocess.run(["git", "-C", root, "rev-parse", "HEAD"],
                             capture_output=True, text=True, timeout=10)
        if out.returncode == 0:
            return out.stdout.strip()
    except Exception:
        pass
    return "unknown"


def gate(endpoint):
    m = rpc(endpoint, "ledger.getMomentumsByHeight", [1, 1])
    h = m["list"][0]
    if h.get("chainIdentifier") != MAINNET_CHAIN_ID:
        raise SystemExit(f"REFUSING: {endpoint} reports chain identifier "
                         f"{h.get('chainIdentifier')}, expected {MAINNET_CHAIN_ID}")
    if h.get("hash") != MAINNET_GENESIS_MOMENTUM:
        raise SystemExit(f"REFUSING: {endpoint} reports momentum-1 hash "
                         f"{h.get('hash')}, expected {MAINNET_GENESIS_MOMENTUM}")


def scan_headers(endpoint, to_h, fh):
    """Walk momentum headers 1..to_h in pages, verifying hash links."""
    read = 0
    prev_hash = None
    h = 1
    last_h = 0
    link_breaks = []
    while h <= to_h:
        page = rpc(endpoint, "ledger.getMomentumsByHeight", [h, PAGE])["list"]
        if not page:
            break
        for i, m in enumerate(page):
            if prev_hash is not None and m.get("previousHash") != prev_hash:
                link_breaks.append({"atHeight": m.get("height"),
                                    "previousHash": m.get("previousHash"),
                                    "expected": prev_hash})
            prev_hash = m.get("hash")
            read += 1
        last_h = page[-1].get("height")
        if last_h - h + 1 != len(page):
            link_breaks.append({"detail": "non-contiguous page", "from": h,
                                "to": last_h, "entries": len(page)})
        if read % PROGRESS_EVERY < PAGE:
            log({"case": "M-3-headers", "status": "INFO", "progress": read,
                 "atHeight": last_h}, fh)
        if len(page) < PAGE:
            break
        h = last_h + 1
    log({"case": "M-3-headers", "status": "PASS" if not link_breaks else "FAIL",
         "momentumsRead": read, "toHeight": prev_hash and last_h,
         "linkBreaks": link_breaks[:20], "linkBreakCount": len(link_breaks)}, fh)
    return read, link_breaks


def scan_blocks(endpoint, from_h, to_h, fh):
    """Scan full account blocks over a height range."""
    momentums = 0
    blocks_read = 0
    bridge_keys = []
    high_diff = []
    h = from_h
    last_h = from_h - 1
    while h <= to_h:
        page = rpc(endpoint, "ledger.getDetailedMomentumsByHeight",
                   [h, DETAILED]).get("list") or []
        if not page:
            break
        for e in page:
            m = e["momentum"]
            momentums += 1
            try:
                mdiff = int(m.get("difficulty", "0"))
            except (ValueError, TypeError):
                mdiff = 0
            if mdiff >= HIGH_DIFF_THRESHOLD:
                high_diff.append({"kind": "momentum", "height": m.get("height"),
                                  "hash": m.get("hash"), "difficulty": m.get("difficulty")})
            for b in e.get("blocks") or []:
                blocks_read += 1
                try:
                    bdiff = int(b.get("difficulty", "0") or 0)
                except (ValueError, TypeError):
                    bdiff = 0
                if bdiff >= HIGH_DIFF_THRESHOLD:
                    high_diff.append({"kind": "block", "height": m.get("height"),
                                      "hash": b.get("hash"),
                                      "difficulty": b.get("difficulty")})
                data = b.get("data") or ""
                raw = b""
                try:
                    raw = base64.b64decode(data)
                except Exception:
                    pass
                if raw[:4] == BRIDGE_KEY_SELECTOR:
                    bridge_keys.append({"height": m.get("height"),
                                        "momentumHash": m.get("hash"),
                                        "address": b.get("address"),
                                        "blockHash": b.get("hash"),
                                        "selector": BRIDGE_KEY_SELECTOR.hex()})
        last_h = page[-1]["momentum"].get("height")
        if momentums % PROGRESS_EVERY < DETAILED:
            log({"case": "M-3-blocks", "status": "INFO", "momentums": momentums,
                 "blocks": blocks_read, "atHeight": last_h}, fh)
        if len(page) < DETAILED:
            break
        h = last_h + 1
    for k in bridge_keys:
        log({"case": "M-3-bridgeKey", **k}, fh)
    for d in high_diff:
        log({"case": "M-3-highDifficulty", **d}, fh)
    log({"case": "M-3-blocks", "status": "PASS", "momentumsRead": momentums,
         "accountBlocksRead": blocks_read, "toHeight": last_h,
         "bridgeKeySends": len(bridge_keys),
         "highDifficultyItems": len(high_diff)}, fh)
    return momentums, blocks_read, bridge_keys


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--node", default="http://127.0.0.1:35997")
    ap.add_argument("--headers", action="store_true", help="walk + link-check headers 1..--to")
    ap.add_argument("--blocks", action="store_true", help="scan full blocks --from..--to")
    ap.add_argument("--from", dest="from_h", type=int, default=1)
    ap.add_argument("--to", dest="to_h", type=int, default=0,
                    help="stated end height (0 = current frontier)")
    ap.add_argument("--out", default="")
    args = ap.parse_args()

    if not args.headers and not args.blocks:
        sys.exit("Specify --headers and/or --blocks")

    path = args.out or os.path.join(os.environ.get("TMPDIR", "/tmp"), "scan-results.jsonl")
    gate(args.node)
    frontier = rpc(args.node, "ledger.getFrontierMomentum")["height"]
    to_h = args.to_h or frontier
    with open(path, "w") as fh:
        log({"case": "header", "tool": "test/mainnet/scan.py",
             "toolCommit": tool_commit(),
             "startedUtc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
             "node": args.node, "frontier": frontier, "to": to_h,
             "bridgeKeySelector": BRIDGE_KEY_SELECTOR.hex(),
             "bridgeKeySig": BRIDGE_KEY_SIG}, fh)
        log({"case": "gate", "status": "PASS",
             "detail": "chain id 1 and mainnet genesis momentum verified"}, fh)
        fail = 0
        if args.headers:
            _, breaks = scan_headers(args.node, min(to_h, frontier), fh)
            fail += len(breaks)
        if args.blocks:
            if to_h > frontier:
                sys.exit(f"--to {to_h} is above the frontier {frontier}")
            scan_blocks(args.node, args.from_h, to_h, fh)
        log({"case": "summary", "fail": fail}, fh)
    if not args.out:
        with open(path) as fh:
            sys.stdout.write(fh.read())
    else:
        print(f"Wrote {path}", file=sys.stderr)
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    main()
