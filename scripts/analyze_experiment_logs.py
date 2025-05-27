#!/usr/bin/env python3
"""
Causal‑ordering experiment analyser – **final fixed version**
===========================================================
* Reconstructs the expected publication order per **publisherID** from the
  global monotonically‑increasing data‑id (``seq`` in *message_received* lines).
* Uses the **earliest on‑chain ordering_key** for each message to build the
  actual receive order, eliminating duplicates that appear later in the log.
* All other plots (throughput, precision) are unchanged.

Run exactly as before:
```
./analyze_fixed.py /path/to/log/dir
```
"""

# ── imports ───────────────────────────────────────────────────────────────────
import argparse
import glob
import json
import os
import re

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from scipy.stats import kendalltau

# ─── Global styling ───────────────────────────────────────────────────────────
plt.style.use("ggplot")
plt.rcParams.update(
    {
        "axes.titlesize": 14,
        "axes.labelsize": 12,
        "xtick.labelsize": 10,
        "ytick.labelsize": 10,
        "figure.dpi": 300,
    }
)
COLOR_CYCLE = plt.get_cmap("tab10").colors

# ─── helper styling fn ────────────────────────────────────────────────────────

def beautify(ax):
    ax.grid(True, linestyle="--", linewidth=0.5, alpha=0.7)
    for spine in ("top", "right"):
        ax.spines[spine].set_visible(False)
    ax.tick_params(axis="x", rotation=45)
    return ax

# ─── log‑parsing helpers ─────────────────────────────────────────────────────

def extract_json(line: str):
    """Return the first JSON object found inside *line* (or None)."""
    i, j = line.find("{"), line.rfind("}")
    if i == -1 or j == -1 or j <= i:
        return None
    try:
        return json.loads(line[i : j + 1])
    except json.JSONDecodeError:
        return None


def load_logs(fname: str):
    """Load *fname* and return a list of event dicts.

    A *message_received* event is split across two consecutive JSON lines – we
    merge them so the resulting record contains both ``publisherID`` and
    ``ordering_key``.
    """
    recs = []
    pending = None  # first half of a receive event

    with open(fname, "r", encoding="utf‑8") as fh:
        for line in fh:
            rec = extract_json(line)
            if not rec:
                continue

            if rec.get("event") == "message_received":
                pending = rec.copy()
                continue

            if "publisherID" in rec and pending is not None:
                pending["publisherID"] = rec["publisherID"]
                recs.append(pending)
                pending = None
                continue

            # one‑liner event
            recs.append(rec)

    return recs

# ─── dataframe prep ──────────────────────────────────────────────────────────

def prepare_dataframe(recs):
    df = pd.DataFrame(recs)
    if "timestamp" in df.columns:
        df["timestamp"] = pd.to_datetime(df["timestamp"], errors="coerce")
    return df

# ─── plotting helpers (precision + throughput unchanged) ─────────────────────

def plot_aggregated_publish_rate(df: pd.DataFrame, size: int):
    summary = df[df["event"] == "experiment_summary"]
    if summary.empty:
        return
    last = summary.sort_values("timestamp").groupby("node", as_index=False).last()
    fig, ax = plt.subplots(figsize=(8, 4))
    bars = ax.bar(
        last["node"].astype(str),
        last["avg_publish_rate"].astype(float),
        color=COLOR_CYCLE[: len(last)],
    )
    ax.set_yscale("log")
    ax.set_xlabel("Node")
    ax.set_ylabel("Avg Publish Rate (msg/sec)")
    ax.set_title(f"Publish Rate per Node ({size} nodes)")
    beautify(ax)
    for b in bars:
        ax.annotate(
            f"{b.get_height():.2f}",
            (b.get_x() + b.get_width() / 2, b.get_height()),
            textcoords="offset points",
            xytext=(0, 3),
            ha="center",
            va="bottom",
            fontsize=9,
        )
    fig.tight_layout()
    fig.savefig(f"plots/throughput_publish_rate_{size}.pdf", bbox_inches="tight")
    plt.close(fig)


def plot_ordering_precision_bar(df: pd.DataFrame, size: int):
    df_pub = df[df["event"] == "message_published"]
    df_dep = df_pub[df_pub["dependencies"].apply(lambda x: isinstance(x, list) and len(x) > 0)]
    if df_dep.empty:
        return
    df_dep["seq"] = pd.to_numeric(df_dep["seq"], errors="coerce")
    precisions = []
    for node, grp in df_dep.groupby("node"):
        deltas = grp.sort_values("seq")["timestamp"].diff().dt.total_seconds().dropna()
        pos = deltas[deltas > 0]
        precisions.append((node, pos.min() * 1e6 if not pos.empty else np.nan))
    bar_df = pd.DataFrame(precisions, columns=["node", "precision_us"]).sort_values("node")
    fig, ax = plt.subplots(figsize=(8, 4))
    bars = ax.barh(bar_df["node"].astype(str), bar_df["precision_us"], color=COLOR_CYCLE[: len(bar_df)])
    ax.set_xlabel("Ordering Precision (μs)")
    ax.set_title(f"Precision per Node ({size} nodes)")
    beautify(ax)
    for b in bars:
        ax.annotate(f"{b.get_width():.1f} μs", (b.get_width(), b.get_y() + b.get_height() / 2),
                    textcoords="offset points", xytext=(3, 0), va="center", fontsize=9)
    fig.tight_layout()
    fig.savefig(f"plots/ordering_precision_bar_{size}.pdf", bbox_inches="tight")
    plt.close(fig)


def plot_ordering_precision_hist(df: pd.DataFrame, size: int):
    df_pub = df[df["event"] == "message_published"]
    df_dep = df_pub[df_pub["dependencies"].apply(lambda x: isinstance(x, list) and len(x) > 0)]
    if df_dep.empty:
        return
    df_dep["seq"] = pd.to_numeric(df_dep["seq"], errors="coerce")
    diffs = (
        df_dep.sort_values(["node", "seq"])
        .groupby("node")["timestamp"]
        .diff()
        .dt.total_seconds()
        .dropna()
    )
    diffs = diffs[diffs > 0]
    if diffs.empty:
        return
    mn, md, m = diffs.min(), diffs.median(), diffs.mean()
    fig, ax = plt.subplots(figsize=(8, 4))
    ax.hist(diffs, bins=40, edgecolor="black")
    for val, style, lab in ((mn, "--", "min"), (md, "-.", "median"), (m, ":", "mean")):
        ax.axvline(val, linestyle=style, label=f"{lab}={val:.6f}s")
    ax.set_xlabel("Δ time between msgs (s)")
    ax.set_ylabel("Frequency")
    ax.set_title(f"Precision Histogram ({size} nodes)")
    ax.legend(fontsize=8)
    beautify(ax)
    fig.tight_layout()
    fig.savefig(f"plots/ordering_precision_hist_{size}.pdf", bbox_inches="tight")
    plt.close(fig)

# ─── ordering consistency (per‑publisher) ─────────────────────────────────────

def plot_ordering_consistency(df: pd.DataFrame, size: int):
    df_rec = df[df["event"] == "message_received"].dropna(subset=["publisherID"])
    if df_rec.empty:
        return

    df_rec["seq"] = pd.to_numeric(df_rec["seq"], errors="coerce")
    df_rec["ordering_key"] = pd.to_numeric(df_rec["ordering_key"], errors="coerce")
    df_rec["publisherID"] = df_rec["publisherID"].astype(str)

    # expected order: unique list sorted by seq
    expected = (
        df_rec[["publisherID", "seq"]]
        .drop_duplicates()
        .sort_values(["publisherID", "seq"])
        .groupby("publisherID")["seq"]
        .apply(list)
        .to_dict()
    )

    # actual order: earliest ordering_key per message, then ordering_key sort
    ok_min = (
        df_rec.dropna(subset=["ordering_key"])
        .groupby(["publisherID", "seq"], as_index=False)["ordering_key"].min()
    )
    actual = (
        ok_min.sort_values(["publisherID", "ordering_key"])
        .groupby("publisherID")["seq"]
        .apply(list)
        .to_dict()
    )

    scores = []
    for pub, exp_seq in expected.items():
        recd = actual.get(pub, [])
        rank = {s: i for i, s in enumerate(exp_seq)}
        idxs = [rank[s] for s in recd if s in rank]
        if len(idxs) >= 2:
            tau, _ = kendalltau(range(len(idxs)), idxs)
            scores.append((pub, (tau + 1) / 2 * 100))
    if not scores:
        return

    cons_df = pd.DataFrame(scores, columns=["node", "consistency"]).sort_values("node")
    fig, ax = plt.subplots(figsize=(8, 4))
    bars = ax.barh(cons_df["node"].astype(str), cons_df["consistency"], color=COLOR_CYCLE[: len(cons_df)])
    ax.set_xlabel("Ordering Consistency (%)")
    ax.set_title(f"Consistency per Publisher ({size} nodes)")
    beautify(ax)
    for b in bars:
        ax.annotate(
        f"{b.get_width():.1f}%",
        (b.get_width(), b.get_y() + b.get_height() / 2),
        textcoords="offset points",
        xytext=(3, 0),
        va="center",
        fontsize=9,
        )

    fig.tight_layout()
    fig.savefig(f"plots/ordering_consistency_{size}.pdf", bbox_inches="tight")
    plt.close(fig)

# ── Main driver ──────────────────────────────────────────────────────────────
def main() -> None:
    ap = argparse.ArgumentParser(description="Analyse logs and produce plots")
    ap.add_argument("logdir", help="directory containing experiment_results_*.log files")
    args = ap.parse_args()

    os.makedirs("plots", exist_ok=True)
    print(f"Log directory: {args.logdir}")
    # discover log files
    files = []
    for f in glob.glob(os.path.join(args.logdir, "experiment_results_*.log")):
        print(f)
        m = re.search(r"experiment_results_(\d+)\.log$", os.path.basename(f))
        if m:
            files.append((int(m.group(1)), f))
    files.sort(key=lambda x: x[0])  # sort by cluster size
    
    print(f"Found {len(files)} log files")
    throughput_summary, precision_summary, consistency_summary = [], [], []

    for size, path in files:
        print(f"Processing cluster size {size}: {path}")
        recs = load_logs(path)
        if not recs:
            print("  ⚠️  no JSON events; skipped")
            continue

        df = prepare_dataframe(recs)
        df["ordering_key"] = pd.to_numeric(df.get("ordering_key", np.nan), errors="coerce")
        df["seq"] = pd.to_numeric(df.get("seq", np.nan), errors="coerce")

        # per-node plots ------------------------------------------------------
        plot_aggregated_publish_rate(df, size)
        plot_ordering_precision_bar(df, size)
        plot_ordering_precision_hist(df, size)
        plot_ordering_consistency(df, size)

        # throughput summary --------------------------------------------------
        exp = df[df["event"] == "experiment_summary"]
        if not exp.empty:
            last = exp.sort_values("timestamp").groupby("node", as_index=False).last()
            total = last["messages_published"].astype(float).sum()
            duration = last["run_duration_sec"].astype(float).max()
            throughput_summary.append((size, total / duration))

        # precision summary ---------------------------------------------------
        pub = df[df["event"] == "message_published"]
        dep = pub[pub["dependencies"].apply(lambda x: isinstance(x, list) and len(x) > 0)]
        if not dep.empty:
            diffs = (
                dep.sort_values(["node", "seq", "timestamp"])
                .groupby("node")["timestamp"]
                .diff()
                .dt.total_seconds()
                .dropna()
            )
            diffs = diffs[diffs > 0]
            if not diffs.empty:
                precision_summary.append(
                    (size, diffs.min() * 1e6, diffs.median() * 1e6, diffs.max() * 1e6)
                )

        # consistency summary -------------------------------------------------
        rec = df[df["event"] == "message_received"].dropna(subset=["publisherID"])
        if not rec.empty:
            rec["seq"] = pd.to_numeric(rec["seq"], errors="coerce")
            rec["ordering_key"] = pd.to_numeric(rec["ordering_key"], errors="coerce")
            rec["publisherID"] = rec["publisherID"].astype(str)

            # expected
            expected = (
                rec[["publisherID", "seq"]]
                .drop_duplicates()
                .sort_values(["publisherID", "seq"])
                .groupby("publisherID")["seq"]
                .apply(list)
                .to_dict()
            )

            # actual (earliest ordering_key per message)
            ok_min = (
                rec.dropna(subset=["ordering_key"])
                .groupby(["publisherID", "seq"], as_index=False)["ordering_key"]
                .min()
            )
            actual = (
                ok_min.sort_values(["publisherID", "ordering_key"])
                .groupby("publisherID")["seq"]
                .apply(list)
                .to_dict()
            )

            vals = []
            for pub, exp_list in expected.items():
                act_list = actual.get(pub, [])
                rank = {s: i for i, s in enumerate(exp_list)}
                idxs = [rank[s] for s in act_list if s in rank]
                if len(idxs) >= 2:
                    tau, _ = kendalltau(range(len(idxs)), idxs)
                    vals.append((tau + 1) / 2 * 100)
            if vals:
                consistency_summary.append((size, np.mean(vals)))

    # ─ summary plots ─────────────────────────────────────────────────────────
    throughput_summary.sort(key=lambda x: x[0])
    precision_summary.sort(key=lambda x: x[0])
    consistency_summary.sort(key=lambda x: x[0])

    # throughput vs size
    if throughput_summary:
        sizes, rates = zip(*throughput_summary)
        fig, ax = plt.subplots(figsize=(6, 4))
        ax.plot(sizes, rates, marker="o", color=COLOR_CYCLE[0])
        ax.set_xlabel("Cluster Size")
        ax.set_ylabel("Throughput (msg/sec)")
        ax.set_title("Total Publish Throughput vs Cluster Size")
        beautify(ax)
        for x, y in zip(sizes, rates):
            ax.annotate(f"{y:.1f}", (x, y), xytext=(0, 3),
                        textcoords="offset points", ha="center")
        fig.tight_layout()
        fig.savefig("plots/total_throughput_by_size.pdf", bbox_inches="tight")
        plt.close(fig)

    # precision vs size
    if precision_summary:
        sizes, mins, meds, maxs = zip(*precision_summary)
        lower = np.array(meds) - np.array(mins)
        upper = np.array(maxs) - np.array(meds)
        fig, ax = plt.subplots(figsize=(6, 4))
        ax.errorbar(
            sizes, meds, yerr=[lower, upper],
            fmt="o", capsize=5, color=COLOR_CYCLE[1]
        )
        ax.set_xlabel("Cluster Size")
        ax.set_ylabel("Precision (μs)")
        ax.set_title("Ordering Precision vs Cluster Size")
        beautify(ax)
        fig.tight_layout()
        fig.savefig("plots/precision_vs_size.pdf", bbox_inches="tight")
        plt.close(fig)

    # consistency vs size
        # consistency vs size
        # consistency vs size  ─ sparse labels
    if consistency_summary:
        sizes, vals = zip(*consistency_summary)

        fig, ax = plt.subplots(figsize=(7, 4))
        bars = ax.bar(
            sizes, vals, width=4,
            color="#4CAF50", edgecolor="black", linewidth=0.6
        )

        ax.set_ylim(99.5, 100.5)
        ax.set_xlabel("Cluster Size", fontweight="bold")
        ax.set_ylabel("Mean Ordering Consistency (%)", fontweight="bold")
        ax.set_title(
            "Causal Ordering Consistency vs Cluster Size",
            fontsize=14, fontweight="semibold", pad=12,
        )
        beautify(ax)

        step = max(len(sizes) // 8, 1)  # label ~8 bars max
        for idx, (b, v) in enumerate(zip(bars, vals)):
            if idx in (0, len(bars) - 1):        # first OR last bar only
                ax.text(
                    b.get_x() + b.get_width() / 2,
                    v + 0.05,
                    f"{v:.1f} %",
                    ha="center",
                    va="bottom",
                    fontsize=9,
                )

        ax.axhline(100, color="gray", linestyle="--", linewidth=0.8)
        fig.tight_layout()
        fig.savefig("plots/ordering_consistency_vs_size.pdf", bbox_inches="tight")
        plt.close(fig)

    print("Analysis complete. PDFs saved to plots/")

# -----------------------------------------------------------------------------


if __name__ == "__main__":
    main()

