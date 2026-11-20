#!/usr/bin/env python3
"""Pointbreak benchmark: the same read-only tasks, with and without Pointbreak, scored against a fixed answer key.

Conditions
  baseline   : the four repos only. Tools: Read, Grep, Glob.
  pointbreak : the same repos + context/ (examples/kitebox) + a workspace CLAUDE.md that imports the rules
               + the pointbreak MCP server. Tools: Read, Grep, Glob, mcp__pointbreak__*.
Both: same model, same prompts, no edits allowed, no other MCP servers, fresh session per run.

Usage: python3 run.py [--repeats 2] [--model sonnet] [--tasks t1,t2,t3]
Writes results/raw-<timestamp>.jsonl and results/summary-<timestamp>.md.
"""
import argparse, datetime, json, os, re, shutil, statistics, subprocess, sys, time

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
WORK = os.path.join(HERE, "work")
RESULTS = os.path.join(HERE, "results")

TASKS = {
    "t1": {
        "name": "Impact of an event change",
        "prompt": "We need to add a `refund_reason` field to the `payment.refunded` event. Without editing anything, "
                  "list every repo and file that must change or be checked, and the order to roll the change out in.",
        "keys": {
            "event contract (payment.proto)": r"payment\.proto",
            "publisher in payments-svc": r"nats/publisher\.go",
            "orders-svc consumer": r"events/consumer\.go",
            "notifications-svc consumer": r"payment_refunded\.go",
            "refund email template": r"refund\.tmpl|email/refund",
            "refund reason source in payments core": r"core/refund\.go|core/types\.go|OnRefundSucceeded",
            "rollout starts with the contract": r"(proto|contract)[^\n]{0,200}\b(first|before)\b|\bfirst\b[^\n]{0,160}(proto|contract)|^\s*1[.)][^\n]{0,80}(proto|contract)",
        },
    },
    "t2": {
        "name": "Schema question",
        "prompt": "Which tables does payments-svc own? Which column links a refund to its charge, and what stops a "
                  "refund from exceeding what was charged? Answer with file references, without editing anything.",
        "keys": {
            "charges": r"\bcharges\b",
            "refunds": r"\brefunds\b",
            "payment_methods": r"payment_methods",
            "refunds.charge_id": r"charge_id",
            "refunded_amount": r"refunded_amount",
            "database check constraint": r"refunded_le_amount|check constraint|CHECK \(",
            "application check in core/refund.go": r"refund\.go",
        },
    },
    "t3": {
        "name": "Cross-service flow",
        "prompt": "A customer cancels an order they already paid for. Describe the whole flow across services: every "
                  "gRPC call, event, database write and customer message, in order. Don't edit anything.",
        "keys": {
            "CancelOrder entry point": r"CancelOrder",
            "Payments.Refund gRPC call": r"Payments[./]Refund|Refund\s*\(?\s*RPC|`?Refund`?\s+(gRPC|RPC|call)",
            "Stripe webhook confirms refund": r"webhook",
            "payment.refunded event": r"payment\.refunded",
            "order.cancelled event": r"order\.cancelled",
            "status history row (order_status_events)": r"order_status_events",
            "charges.refunded_amount update": r"refunded_amount",
            "cancellation SMS": r"\bSMS\b",
            "refund email": r"email/refund|refund\.tmpl|refund (email|notice)",
            "order marked refunded (MarkRefunded)": r"MarkRefunded",
        },
    },
}

BLOCK = """<!-- pointbreak:start -->
## Pointbreak
Team rules for agents: @context/README.md
Per-service memory lives in context/<service>/. Start work with /pointbreak <repos> [branches] [inputs…].
<!-- pointbreak:end -->
"""


def sh(cmd, **kw):
    return subprocess.run(cmd, check=True, capture_output=True, text=True, **kw)


def prepare(binary):
    if os.path.exists(WORK):
        shutil.rmtree(WORK)
    envs = {}
    for cond in ("baseline", "pointbreak"):
        ws = os.path.join(WORK, cond)
        sh([sys.executable, os.path.join(HERE, "make_fixture.py"), ws])
        for repo in sorted(os.listdir(ws)):
            sh(["git", "init", "-q"], cwd=os.path.join(ws, repo))
        mcp = {"mcpServers": {}}
        if cond == "pointbreak":
            shutil.copytree(os.path.join(ROOT, "examples", "kitebox", "context"), os.path.join(ws, "context"))
            with open(os.path.join(ws, "CLAUDE.md"), "w") as fh:
                fh.write(BLOCK)
            mcp["mcpServers"]["pointbreak"] = {"command": binary, "args": ["--workspace", ws]}
        cfg = os.path.join(WORK, f"mcp-{cond}.json")
        with open(cfg, "w") as fh:
            json.dump(mcp, fh)
        envs[cond] = (ws, cfg)
    return envs


def run_one(cond, ws, cfg, task, model, budget):
    tools = "Read,Grep,Glob" + (",mcp__pointbreak" if cond == "pointbreak" else "")
    cmd = ["claude", "-p", task["prompt"], "--output-format", "json", "--model", model,
           "--no-session-persistence", "--strict-mcp-config", "--mcp-config", cfg,
           "--allowedTools", tools,
           "--disallowedTools", "Edit,Write,MultiEdit,NotebookEdit,Bash,WebFetch,WebSearch,Task,Agent",
           "--max-budget-usd", str(budget)]
    t0 = time.time()
    p = subprocess.run(cmd, cwd=ws, capture_output=True, text=True, timeout=900)
    wall = time.time() - t0
    try:
        out = json.loads(p.stdout)
    except json.JSONDecodeError:
        out = {"is_error": True, "result": (p.stdout + p.stderr)[-2000:]}
    answer = out.get("result") or ""
    hits = {k: bool(re.search(rx, answer, re.I | re.M)) for k, rx in task["keys"].items()}
    u = out.get("usage") or {}
    return {
        "condition": cond, "task": task["id"], "model": model,
        "score": sum(hits.values()), "max": len(hits), "hits": hits,
        "input_tokens": u.get("input_tokens", 0), "cache_creation_tokens": u.get("cache_creation_input_tokens", 0),
        "cache_read_tokens": u.get("cache_read_input_tokens", 0), "output_tokens": u.get("output_tokens", 0),
        "cost_usd": out.get("total_cost_usd"), "turns": out.get("num_turns"), "duration_s": round(wall, 1),
        "is_error": out.get("is_error", p.returncode != 0), "answer": answer,
    }


def summarise(rows, path, model, repeats):
    def m(xs):
        return statistics.mean(xs) if xs else 0
    lines = [f"# Pointbreak benchmark — {datetime.date.today()}", "",
             f"Model `{model}`, {repeats} run(s) per task per condition, read-only tasks, fresh session each run. "
             "Method and answer keys: `bench/run.py`. Raw answers: the matching `raw-*.jsonl`.", "",
             "| Task | Condition | Answer-key score | Total input tokens | of which cache reads | Output tokens | Turns | Time (s) | Cost (USD) |",
             "|---|---|---|---|---|---|---|---|---|"]
    for tid, task in TASKS.items():
        for cond in ("baseline", "pointbreak"):
            rs = [r for r in rows if r["task"] == tid and r["condition"] == cond and not r["is_error"]]
            if not rs:
                continue
            tin = [r["input_tokens"] + r["cache_creation_tokens"] + r["cache_read_tokens"] for r in rs]
            lines.append(f"| {task['name']} | {cond} | {m([r['score'] for r in rs]):.1f} / {rs[0]['max']} | {m(tin):,.0f} | "
                         f"{m([r['cache_read_tokens'] for r in rs]):,.0f} | {m([r['output_tokens'] for r in rs]):,.0f} | "
                         f"{m([r['turns'] or 0 for r in rs]):.1f} | {m([r['duration_s'] for r in rs]):.0f} | "
                         f"{m([r['cost_usd'] or 0 for r in rs]):.3f} |")
    lines += ["", "## Answer-key items missed", ""]
    for tid, task in TASKS.items():
        for cond in ("baseline", "pointbreak"):
            missed = {}
            for r in rows:
                if r["task"] == tid and r["condition"] == cond and not r["is_error"]:
                    for k, v in r["hits"].items():
                        if not v:
                            missed[k] = missed.get(k, 0) + 1
            if missed:
                lines.append(f"- {task['name']}, {cond}: " + "; ".join(f"{k} ({n}×)" for k, n in missed.items()))
    errs = [r for r in rows if r["is_error"]]
    if errs:
        lines += ["", f"{len(errs)} run(s) errored and are excluded from the table."]
    with open(path, "w") as fh:
        fh.write("\n".join(lines) + "\n")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--repeats", type=int, default=2)
    ap.add_argument("--model", default="sonnet")
    ap.add_argument("--tasks", default="t1,t2,t3")
    ap.add_argument("--budget", type=float, default=1.0, help="max USD per run")
    a = ap.parse_args()
    os.makedirs(RESULTS, exist_ok=True)
    binary = os.path.join(WORK + "-bin", "pointbreak-mcp")
    os.makedirs(os.path.dirname(binary), exist_ok=True)
    sh(["go", "build", "-o", binary, "./cmd/pointbreak-mcp"], cwd=ROOT)
    envs = prepare(binary)
    stamp = datetime.datetime.now().strftime("%Y%m%d-%H%M")
    raw = os.path.join(RESULTS, f"raw-{stamp}.jsonl")
    rows = []
    with open(raw, "w") as fh:
        for rep in range(a.repeats):
            for tid in a.tasks.split(","):
                task = dict(TASKS[tid], id=tid)
                order = ("baseline", "pointbreak") if rep % 2 == 0 else ("pointbreak", "baseline")
                for cond in order:
                    ws, cfg = envs[cond]
                    r = run_one(cond, ws, cfg, task, a.model, a.budget)
                    r["repeat"] = rep
                    rows.append(r)
                    fh.write(json.dumps(r) + "\n")
                    fh.flush()
                    print(f"[{rep}] {tid} {cond:10s} score {r['score']}/{r['max']} turns {r['turns']} "
                          f"in {r['input_tokens'] + r['cache_creation_tokens'] + r['cache_read_tokens']:,} out {r['output_tokens']:,} "
                          f"${r['cost_usd']} {r['duration_s']}s{' ERROR' if r['is_error'] else ''}", flush=True)
    summary = os.path.join(RESULTS, f"summary-{stamp}.md")
    summarise(rows, summary, a.model, a.repeats)
    print("summary:", summary)


if __name__ == "__main__":
    main()
