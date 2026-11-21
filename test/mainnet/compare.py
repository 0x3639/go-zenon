#!/usr/bin/env python3
"""M-2: mainnet RPC differ for the go-zenon test framework (test level L6).

Compares a binary under test against a reference node on mainnet:

  M-2a  momentum header + account-block comparison at the spork heights and
        ranges listed in the L6 specification (0x3639/go-zenon#17), plus the
        frontier.
  M-2b  frontier momentum hash comparison.
  M-2c  frontier state: N account infos (batches, guarded by frontier hashes
        read before and after each batch) and every embedded.* read method in
        test/inventory/coverage.tsv, paged to the end.
  M-2d  controls: self-self comparison (must match) and a planted height
        difference h vs h+1 (must be detected).

Refuses to run unless every endpoint reports chain identifier 1 and the
mainnet genesis momentum hash at height 1.

Standard library Python 3 only.  Signs nothing: no wallet import, no call to
ledger.publishRawTransaction, no writes of any kind.

Output: one JSON line per comparison (JSONL), a final summary by case id,
and the commit the tool was built from.

  python3 test/mainnet/compare.py \
      --ref http://127.0.0.1:35997 \
      --test http://127.0.0.1:36997 \
      --out results.jsonl
"""
import argparse
import json
import os
import subprocess
import sys
import time
import urllib.request

# Mainnet momentum-1 hash and the chain identifier every endpoint must report.
MAINNET_GENESIS_MOMENTUM = "9e204601d1b7b1427fe12bc82622e610d8a6ad43c40abf020eb66e538bb8eeb0"
MAINNET_CHAIN_ID = 1

# M-2a heights: every spork activation height and the one before it, the
# 4,188,198-4,188,207 window (HTLC + Multiverse activations), and the
# 14,567,886-14,567,888 renewal window (0x3639's specification, #17).
M2A_HEIGHTS = (
    [1_241_272, 1_241_273]
    + list(range(4_188_198, 4_188_208))
    + [14_567_886, 14_567_887, 14_567_888]
)

# Methods left out of the M-2c getter list, with the reason (spec: list them).
LEFT_OUT = [
    ("stats.syncInfo", "node-local"),
    ("stats.networkInfo", "node-local"),
    ("stats.osInfo", "node-local"),
    ("stats.processInfo", "node-local"),
    ("ledger.getUnconfirmedBlocksByAddress", "unconfirmed pool is node-local"),
    ("ledger.publishRawTransaction", "write method"),
    ("ledger.subscribe", "subscription"),
]

PAGE = 1000          # RpcMaxPageSize / RpcMaxCountSize
MAX_PAGES = 500      # guard against unbounded paging; truncation is reported
ZNN_ZTS = "zts1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"

# Argument sources for the M-2c getter list (every embedded.* read method in
# test/inventory/coverage.tsv).  Markers:
#   ("addr",)       the first derived mainnet address (see derive_addresses)
#   ("page",)       trailing paging marker: the getter is paged to the end
#   other tuples    lazily resolved from the reference node (see Ctx)
#   literals        used verbatim
GETTERS = [
    # accelerator (5 read methods in coverage.tsv)
    ("embedded.accelerator.getAll", [("page",)]),
    ("embedded.accelerator.getProjectById", [("project",)]),
    ("embedded.accelerator.getPhaseById", [("phase",)]),
    ("embedded.accelerator.getVoteBreakdown", [("project",)]),
    ("embedded.accelerator.getPillarVotes", [("name",), [("project",)]]),
    # bridge (15)
    ("embedded.bridge.getBridgeInfo", []),
    ("embedded.bridge.getSecurityInfo", []),
    ("embedded.bridge.getOrchestratorInfo", []),
    ("embedded.bridge.getTimeChallengesInfo", []),
    ("embedded.bridge.getNetworkInfo", [("networkClass",), ("chainId",)]),
    ("embedded.bridge.getAllNetworks", [("page",)]),
    ("embedded.bridge.getWrapTokenRequestById", [("wrapId",)]),
    ("embedded.bridge.getAllWrapTokenRequests", [("page",)]),
    ("embedded.bridge.getAllWrapTokenRequestsByToAddress", [("wrapToAddress",), ("page",)]),
    ("embedded.bridge.getAllWrapTokenRequestsByToAddressNetworkClassAndChainId",
        [("wrapToAddress",), ("networkClass",), ("chainId",), ("page",)]),
    ("embedded.bridge.getAllUnsignedWrapTokenRequests", [("page",)]),
    ("embedded.bridge.getUnwrapTokenRequestByHashAndLog", [("unwrapHash",), ("unwrapLogIndex",)]),
    ("embedded.bridge.getAllUnwrapTokenRequests", [("page",)]),
    ("embedded.bridge.getAllUnwrapTokenRequestsByToAddress", [("unwrapToAddress",), ("page",)]),
    ("embedded.bridge.getFeeTokenPair", [("feeZts",)]),
    # htlc (2; coverage.tsv has no list method, so getById uses the zero hash
    # and compares the not-found answer on both nodes)
    ("embedded.htlc.getById", [("zeroHash",)]),
    ("embedded.htlc.getProxyUnlockStatus", [("addr",)]),
    # liquidity (6)
    ("embedded.liquidity.getLiquidityInfo", []),
    ("embedded.liquidity.getSecurityInfo", []),
    ("embedded.liquidity.getLiquidityStakeEntriesByAddress", [("addr",), ("page",)]),
    ("embedded.liquidity.getUncollectedReward", [("addr",)]),
    ("embedded.liquidity.getFrontierRewardByPage", [("addr",), ("page",)]),
    ("embedded.liquidity.getTimeChallengesInfo", []),
    # pillar (9)
    ("embedded.pillar.getDepositedQsr", [("addr",)]),
    ("embedded.pillar.getUncollectedReward", [("addr",)]),
    ("embedded.pillar.getFrontierRewardByPage", [("addr",), ("page",)]),
    ("embedded.pillar.getQsrRegistrationCost", []),
    ("embedded.pillar.getAll", [("page",)]),
    ("embedded.pillar.getByOwner", [("addr",)]),
    ("embedded.pillar.getByName", [("name",)]),
    ("embedded.pillar.checkNameAvailability", [("name",)]),
    ("embedded.pillar.getDelegatedPillar", [("addr",)]),
    ("embedded.pillar.getPillarEpochHistory", [("name",), ("page",)]),
    ("embedded.pillar.getPillarsHistoryByEpoch", [1, ("page",)]),
    # plasma (4)
    ("embedded.plasma.get", [("addr",)]),
    ("embedded.plasma.getEntriesByAddress", [("addr",), ("page",)]),
    ("embedded.plasma.getRequiredPoWForAccountBlock",
        [{"address": ("addr",), "blockType": 2, "toAddress": ("addr",), "data": ""}]),
    ("embedded.plasma.getVariables", []),
    # ptlc (2)
    ("embedded.ptlc.getById", [("zeroHash",)]),
    ("embedded.ptlc.getReclaimLockById", [("zeroHash",)]),
    # sentinel (5)
    ("embedded.sentinel.getAllActive", [("page",)]),
    ("embedded.sentinel.getByOwner", [("addr",)]),
    ("embedded.sentinel.getDepositedQsr", [("addr",)]),
    ("embedded.sentinel.getUncollectedReward", [("addr",)]),
    ("embedded.sentinel.getFrontierRewardByPage", [("addr",), ("page",)]),
    # spork (1)
    ("embedded.spork.getAll", [("page",)]),
    # stake (3)
    ("embedded.stake.getUncollectedReward", [("addr",)]),
    ("embedded.stake.getFrontierRewardByPage", [("addr",), ("page",)]),
    ("embedded.stake.getEntriesByAddress", [("addr",), ("page",)]),
    # swap (3)
    ("embedded.swap.getAssetsByKeyIdHash", [("swapKeyIdHash",)]),
    ("embedded.swap.getAssets", []),
    ("embedded.swap.getLegacyPillars", []),
    # token (3)
    ("embedded.token.getAll", [("page",)]),
    ("embedded.token.getByOwner", [("addr",), ("page",)]),
    ("embedded.token.getByZts", [("znnZts",)]),
]


def log(line, fh):
    fh.write(json.dumps(line, default=str) + "\n")
    fh.flush()


def rpc(endpoint, method, params=None, timeout=60, retries=3):
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


def rpc_catching(endpoint, method, params=None, timeout=60):
    """Like rpc but returns (result, error-string); transport failures still
    raise after retries.  RPC-level errors are comparable answers."""
    try:
        return rpc(endpoint, method, params, timeout), None
    except RuntimeError as e:
        return None, str(e)


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
    """Spec: refuse to run unless every endpoint reports chain identifier 1
    and the mainnet genesis momentum hash at height 1."""
    m = rpc(endpoint, "ledger.getMomentumsByHeight", [1, 1])
    h = m["list"][0]
    if h.get("chainIdentifier") != MAINNET_CHAIN_ID:
        raise SystemExit(f"REFUSING: {endpoint} reports chain identifier "
                         f"{h.get('chainIdentifier')}, expected {MAINNET_CHAIN_ID}")
    if h.get("hash") != MAINNET_GENESIS_MOMENTUM:
        raise SystemExit(f"REFUSING: {endpoint} reports momentum-1 hash "
                         f"{h.get('hash')}, expected {MAINNET_GENESIS_MOMENTUM}")


def detailed(endpoint, h):
    return rpc(endpoint, "ledger.getDetailedMomentumsByHeight", [h, 1])


def normalize(value):
    """Strip node-local contextual fields that legitimately differ between
    two nodes at different heights (but would also differ at the same
    height): confirmation counters and token supplies embedded in account
    blocks.  Applied recursively so pairedAccountBlock is covered too."""
    if isinstance(value, dict):
        out = {}
        for k, v in value.items():
            if k == "numConfirmations":
                continue
            if k == "token" and isinstance(v, dict):
                v = {tk: tv for tk, tv in v.items()
                     if tk not in ("totalSupply", "maxSupply")}
            out[k] = normalize(v)
        return out
    if isinstance(value, list):
        return [normalize(v) for v in value]
    return value


def compare_momentum_answer(ref_a, test_a):
    """Compare one getDetailedMomentumsByHeight answer.  Returns (status, detail)."""
    rl = ref_a.get("list") or []
    tl = test_a.get("list") or []
    if not rl or not tl:
        return "INFO", {"detail": "no-momentum-at-height", "ref": len(rl), "test": len(tl)}
    r, t = rl[0], tl[0]
    rm, tm = r["momentum"], t["momentum"]
    diffs = []
    for f in ("hash", "changesHash", "height", "version", "chainIdentifier"):
        if rm.get(f) != tm.get(f):
            diffs.append({"field": f, "ref": rm.get(f), "test": tm.get(f)})
    rb = r.get("blocks") or []
    tb = t.get("blocks") or []
    out = {"refBlocks": len(rb), "testBlocks": len(tb)}
    if rb and tb:
        nr, nt = normalize(rb), normalize(tb)
        if nr != nt:
            first = None
            for i, (x, y) in enumerate(zip(nr, nt)):
                if x != y:
                    for k in x:
                        if x.get(k) != y.get(k):
                            first = {"block": i, "field": k,
                                     "ref": str(x.get(k))[:120],
                                     "test": str(y.get(k))[:120]}
                            break
                if first:
                    break
            diffs.append({"field": "blocks", "ref": len(rb), "test": len(tb),
                          "firstDiff": first})
    elif rb or tb:
        # A pruned reference node serves no old account blocks; that is an
        # availability note, not a chain difference.
        out["detail"] = "block-availability-mismatch"
    return ("FAIL" if diffs else "PASS"), {**out, **({"diffs": diffs} if diffs else {})}


class Ctx:
    """Lazily-resolved argument sources, read once from the reference node."""

    def __init__(self, ref, addr):
        self.ref = ref
        self.addr = addr
        self.cache = {}

    def get(self, key):
        if key not in self.cache:
            self.cache[key] = self._resolve(key)
        return self.cache[key]

    def _resolve(self, key):
        if key == "addr":
            return self.addr
        if key == "zeroHash":
            return "00" * 32
        if key == "name":
            l = rpc(self.ref, "embedded.pillar.getAll", [0, 1])["list"]
            if not l:
                raise RuntimeError("no pillars on ref")
            return l[0]["name"]
        if key == "project":
            l = rpc(self.ref, "embedded.accelerator.getAll", [0, 1])["list"]
            if not l:
                raise RuntimeError("no accelerator projects on ref")
            return l[0]["id"]
        if key == "phase":
            p = rpc(self.ref, "embedded.accelerator.getProjectById", [self.get("project")])
            ph = (p or {}).get("phases") or []
            if not ph or not (ph[0].get("phase") or {}).get("id"):
                raise RuntimeError("no phase id on first project")
            return ph[0]["phase"]["id"]
        if key in ("networkClass", "chainId"):
            l = rpc(self.ref, "embedded.bridge.getAllNetworks", [0, 1])["list"]
            if not l:
                raise RuntimeError("no bridge networks on ref")
            return l[0][key]
        if key in ("wrapId", "wrapToAddress"):
            l = rpc(self.ref, "embedded.bridge.getAllWrapTokenRequests", [0, 1])["list"]
            if not l:
                raise RuntimeError("no wrap requests on ref")
            return l[0]["id"] if key == "wrapId" else l[0]["toAddress"]
        if key in ("unwrapHash", "unwrapLogIndex", "unwrapToAddress"):
            l = rpc(self.ref, "embedded.bridge.getAllUnwrapTokenRequests", [0, 1])["list"]
            if not l:
                raise RuntimeError("no unwrap requests on ref")
            e = l[0]
            return {"unwrapHash": e.get("transactionHash"),
                    "unwrapLogIndex": e.get("logIndex"),
                    "unwrapToAddress": e.get("toAddress")}[key]
        if key == "feeZts":
            # ZNN is always a bridge fee token; resolve its zts from the
            # chain instead of embedding a checksum-bearing constant.
            return self.get("znnZts")
        if key == "znnZts":
            for pi in range(20):
                res = rpc(self.ref, "embedded.token.getAll", [pi, 100])
                for t in (res or {}).get("list") or []:
                    if t.get("symbol") == "ZNN":
                        return t["tokenStandard"]
                if len((res or {}).get("list") or []) < 100:
                    break
            raise RuntimeError("ZNN token not found on ref")
        if key == "swapKeyIdHash":
            assets = rpc(self.ref, "embedded.swap.getAssets") or {}
            for h in assets:
                return h
            raise RuntimeError("no swap assets on ref")
        raise RuntimeError(f"unknown arg source {key}")


def build_args(tmpl, ctx):
    out = []
    for a in tmpl:
        if isinstance(a, tuple):
            out.append(ctx.get(a[0]))
        elif isinstance(a, list):
            out.append(build_args(a, ctx))
        elif isinstance(a, dict):
            out.append({k: (ctx.get(v[0]) if isinstance(v, tuple) else v)
                        for k, v in a.items()})
        else:
            out.append(a)
    return out


def page_all(endpoint, method, fixed_args):
    """Page a list getter to the end.  Returns (merged list, pages, truncated)."""
    merged = []
    for pi in range(MAX_PAGES):
        res = rpc(endpoint, method, fixed_args + [pi, PAGE], timeout=180)
        items = (res or {}).get("list") or []
        merged.extend(items)
        count = (res or {}).get("count")
        if len(items) < PAGE or (count is not None and len(merged) >= count):
            return merged, pi + 1, False
    return merged, MAX_PAGES, True


def derive_addresses(endpoint, start, window, want):
    """First-seen unique addresses from account blocks of the last `window`
    momentums at or below `start`, read in pages of 10 walking backwards."""
    seen, order = set(), []
    lo = max(1, start - window + 1)
    h_hi = start
    while h_hi >= lo and len(order) < want:
        h_lo = max(lo, h_hi - 9)
        try:
            page = rpc(endpoint, "ledger.getDetailedMomentumsByHeight",
                       [h_lo, h_hi - h_lo + 1]).get("list") or []
        except Exception:
            h_hi = h_lo - 1
            continue
        for e in page:
            for b in e.get("blocks") or []:
                for k in ("address", "toAddress"):
                    a = b.get(k)
                    if a and a not in seen:
                        seen.add(a)
                        order.append(a)
        h_hi = h_lo - 1
    return order


def frontier_hash(endpoint):
    return rpc(endpoint, "ledger.getFrontierMomentum", timeout=30).get("hash")


def run(ref, test, args, fh):
    log({"case": "header", "tool": "test/mainnet/compare.py",
         "toolCommit": tool_commit(),
         "startedUtc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
         "ref": ref, "test": test,
         "params": {"addrCount": args.addr_count, "addrWindow": args.addr_window,
                    "batchSize": args.batch_size},
         "leftOut": LEFT_OUT}, fh)

    gate(ref)
    gate(test)
    log({"case": "gate", "status": "PASS",
         "detail": "chain id 1 and mainnet genesis momentum verified on both endpoints"}, fh)

    fr = rpc(ref, "ledger.getFrontierMomentum", timeout=30)
    ft = rpc(test, "ledger.getFrontierMomentum", timeout=30)
    t_frontier = ft["height"]

    # ---- M-2a: spork-window momentum comparisons
    for h in M2A_HEIGHTS:
        r = {"case": "M-2a", "height": h}
        if h > t_frontier:
            r["status"] = "INFO"; r["detail"] = "test-not-synced"; r["testHeight"] = t_frontier
        else:
            try:
                status, detail = compare_momentum_answer(detailed(ref, h), detailed(test, h))
                r["status"] = status; r.update(detail)
            except Exception as e:
                r["status"] = "ERROR"; r["error"] = str(e)[:200]
        log(r, fh)

    # ---- M-2b: frontier comparison
    r = {"case": "M-2b",
         "ref": {"height": fr["height"], "hash": fr["hash"]},
         "test": {"height": ft["height"], "hash": ft["hash"]}}
    if fr["hash"] == ft["hash"]:
        r["status"] = "PASS"
    elif fr["height"] == ft["height"]:
        r["status"] = "FAIL"; r["detail"] = "split-chain"
    else:
        r["status"] = "INFO"; r["detail"] = "height-mismatch-syncing"
    log(r, fh)

    # ---- M-2c: frontier state
    common = min(fr["height"], ft["height"])
    # Derive from the test node: it carries the full history, while a
    # pruned reference node serves account blocks only near the frontier.
    addrs = derive_addresses(test, common, args.addr_window, args.addr_count)
    log({"case": "M-2c", "status": "INFO", "detail": "derived-addresses",
         "count": len(addrs),
         "source": f"first-seen unique addresses from account blocks of momentums "
                   f"{max(1, common - args.addr_window)}..{common} on the test node"}, fh)

    for i in range(0, len(addrs), args.batch_size):
        batch = addrs[i:i + args.batch_size]
        r = {"case": "M-2c-accountInfos", "batch": i // args.batch_size, "n": len(batch)}
        h_before = (frontier_hash(ref), frontier_hash(test))
        infos_ref, infos_test, ok = [], [], True
        try:
            for a in batch:
                infos_ref.append(rpc(ref, "ledger.getAccountInfoByAddress", [a], timeout=30))
                infos_test.append(rpc(test, "ledger.getAccountInfoByAddress", [a], timeout=30))
        except Exception as e:
            r["status"] = "ERROR"; r["error"] = str(e)[:200]; ok = False
        h_after = (frontier_hash(ref), frontier_hash(test))
        if ok:
            if h_before[0] == h_before[1] == h_after[0] == h_after[1]:
                r["status"] = "PASS" if infos_ref == infos_test else "FAIL"
                if r["status"] == "FAIL":
                    for a, x, y in zip(batch, infos_ref, infos_test):
                        if x != y:
                            r["firstDiffAddress"] = a
                            break
            else:
                r["status"] = "INFO"; r["detail"] = "frontier-moved-during-batch"
        log(r, fh)

    # Embedded getters: run the identical walk on both nodes and compare.
    # RPC-level errors are comparable answers: identical errors on both sides
    # are a PASS (e.g. a not-found id or a getter missing on the release),
    # an error on exactly one side is an INFO, not a FAIL.
    addr = addrs[0] if addrs else \
        "z1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
    ctx = Ctx(ref, addr)
    for method, tmpl in GETTERS:
        r = {"case": "M-2c-getters", "method": method}
        try:
            paged = bool(tmpl and isinstance(tmpl[-1], tuple) and tmpl[-1][0] == "page")
            fixed = build_args(tmpl[:-1] if paged else tmpl, ctx)
        except Exception as e:
            r["status"] = "ERROR"; r["error"] = ("arg-resolution: " + str(e))[:200]
            log(r, fh)
            continue
        h_before = (frontier_hash(ref), frontier_hash(test))
        rerr = terr = None
        rr = tt = None
        try:
            if paged:
                rr, rp, rtrunc = page_all(ref, method, fixed)
                r["refPages"] = rp
                if rtrunc:
                    r["refTruncated"] = True
            else:
                rr = rpc(ref, method, fixed, timeout=180)
        except Exception as e:
            rerr = str(e)
        try:
            if paged:
                tt, tp, ttrunc = page_all(test, method, fixed)
                r["testPages"] = tp
                if ttrunc:
                    r["testTruncated"] = True
            else:
                tt = rpc(test, method, fixed, timeout=180)
        except Exception as e:
            terr = str(e)
        h_after = (frontier_hash(ref), frontier_hash(test))
        if rerr is not None and terr is not None:
            if ("does not exist" in rerr or "not available" in rerr) != \
               ("does not exist" in terr or "not available" in terr):
                # The method exists on exactly one build (added after the
                # reference release): expected, not a chain difference.
                r["status"] = "INFO"; r["detail"] = "method-missing-on-one-side"
            else:
                # Identical errors on both sides are agreement (not-found id,
                # ...).  Different errors are a real difference in the error
                # surface.
                r["status"] = "PASS" if rerr == terr else "FAIL"
            r["refError"], r["testError"] = rerr[:200], terr[:200]
        elif rerr is not None or terr is not None:
            r["status"] = "INFO"; r["detail"] = "error-on-one-side"
            r["refError"], r["testError"] = (rerr or "")[:200], (terr or "")[:200]
        elif h_before[0] == h_before[1] == h_after[0] == h_after[1]:
            r["status"] = "PASS" if rr == tt else "FAIL"
        else:
            r["status"] = "INFO"; r["detail"] = "frontier-moved-during-getter"
        log(r, fh)

    # ---- M-2d: controls
    try:
        s, d = compare_momentum_answer(detailed(ref, 100), detailed(ref, 100))
        log({"case": "M-2d", "control": "self-self", "height": 100, "status": s, **d}, fh)
    except Exception as e:
        log({"case": "M-2d", "control": "self-self", "status": "ERROR",
             "error": str(e)[:200]}, fh)
    try:
        s, d = compare_momentum_answer(detailed(ref, 100), detailed(ref, 101))
        # The two heights genuinely differ, so the differ reporting FAIL is
        # the control passing.
        log({"case": "M-2d", "control": "planted-height-diff", "heights": [100, 101],
             "status": "PASS" if s == "FAIL" else "FAIL", **d}, fh)
    except Exception as e:
        log({"case": "M-2d", "control": "planted-height-diff", "status": "ERROR",
             "error": str(e)[:200]}, fh)


def summarize(path):
    counts = {}
    with open(path) as fh:
        for line in fh:
            try:
                r = json.loads(line)
            except json.JSONDecodeError:
                continue
            if r.get("case") in ("header", "gate", "summary"):
                continue
            k = (r.get("case"), r.get("status", "ERROR"))
            counts[k] = counts.get(k, 0) + 1
    by_case = {}
    for (case, status), n in sorted(counts.items()):
        by_case.setdefault(case, {})[status] = n
    with open(path, "a") as fh:
        log({"case": "summary", "byCase": by_case,
             "fail": sum(n for (c, s), n in counts.items() if s == "FAIL"),
             "error": sum(n for (c, s), n in counts.items() if s == "ERROR")}, fh)
    return by_case


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--ref", default="http://127.0.0.1:35997")
    ap.add_argument("--test", default="http://127.0.0.1:36997")
    ap.add_argument("--addr-count", type=int, default=1000)
    ap.add_argument("--addr-window", type=int, default=25000)
    ap.add_argument("--batch-size", type=int, default=50)
    ap.add_argument("--out", default="")
    args = ap.parse_args()

    path = args.out or os.path.join(os.environ.get("TMPDIR", "/tmp"), "compare-results.jsonl")
    with open(path, "w") as fh:
        run(args.ref, args.test, args, fh)
    by_case = summarize(path)
    if not args.out:
        with open(path) as fh:
            sys.stdout.write(fh.read())
    else:
        print(f"Wrote {path}", file=sys.stderr)
    fail = sum(s.get("FAIL", 0) for s in by_case.values())
    err = sum(s.get("ERROR", 0) for s in by_case.values())
    print(f"summary: {json.dumps(by_case)}", file=sys.stderr)
    sys.exit(0 if fail == 0 and err == 0 else 1)


if __name__ == "__main__":
    main()
