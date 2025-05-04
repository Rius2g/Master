#!/usr/bin/env python3
import argparse
import json
import sys
import os
import re
import glob
from datetime import datetime

import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
from scipy.stats import kendalltau

# ─── Global styling ────────────────────────────────────────────────────────────
plt.style.use('ggplot')
plt.rcParams.update({
    'axes.titlesize':   14,
    'axes.labelsize':   12,
    'xtick.labelsize':  10,
    'ytick.labelsize':  10,
    'figure.dpi':      300,
})
COLOR_CYCLE = plt.get_cmap('tab10').colors

def beautify(ax):
    """Apply consistent grid, spines, and font styling to an Axes."""
    ax.grid(True, linestyle='--', linewidth=0.5, alpha=0.7)
    for spine in ['top','right']:
        ax.spines[spine].set_visible(False)
    ax.tick_params(axis='x', rotation=45)
    return ax

# ─── JSON log helpers ─────────────────────────────────────────────────────────
def extract_json(line):
    i, j = line.find('{'), line.rfind('}')
    if i == -1 or j == -1 or j <= i:
        return None
    try:
        return json.loads(line[i:j+1])
    except json.JSONDecodeError:
        return None

def load_logs(fname):
    recs = []
    with open(fname, 'r') as f:
        for L in f:
            if r := extract_json(L):
                recs.append(r)
    return recs

def prepare_dataframe(recs):
    df = pd.DataFrame(recs)
    if 'timestamp' in df:
        df['timestamp'] = pd.to_datetime(df['timestamp'], errors='coerce')
    return df

# ─── Plotting functions ───────────────────────────────────────────────────────
def plot_aggregated_publish_rate(df, size):
    summary = df[df["event"] == "experiment_summary"]
    if summary.empty:
        return
    last = summary.sort_values("timestamp").groupby("node", as_index=False).last()
    fig, ax = plt.subplots(figsize=(8,4))
    bars = ax.bar(
        last["node"].astype(str),
        last["avg_publish_rate"].astype(float),
        color=COLOR_CYCLE[:len(last)],
        width=0.8
    )
    ax.set_yscale('log')
    ax.set_xlabel("Node")
    ax.set_ylabel("Avg Publish Rate (msg/sec)")
    ax.set_title(f"Publish Rate per Node ({size} nodes)")
    beautify(ax)
    for b in bars:
        ax.annotate(f"{b.get_height():.2f}",
                    xy=(b.get_x()+b.get_width()/2, b.get_height()),
                    xytext=(0,3), textcoords='offset points',
                    ha='center', va='bottom', fontsize=9)
    fig.tight_layout()
    fig.savefig(f"plots/throughput_publish_rate_{size}.pdf", bbox_inches='tight')
    plt.close(fig)

def plot_ordering_precision_bar(df, size):
    df_pub = df[df['event']=='message_published']
    df_dep = df_pub[df_pub['dependencies'].apply(lambda x: isinstance(x, list) and len(x)>0)]
    if df_dep.empty:
        return
    df_dep['seq'] = pd.to_numeric(df_dep['seq'], errors='coerce')
    precisions = []
    for node, grp in df_dep.groupby('node'):
        grp = grp.sort_values('seq')
        deltas = grp['timestamp'].diff().dt.total_seconds().dropna()
        pos = deltas[deltas>0]
        precision = pos.min()*1e6 if not pos.empty else np.nan
        precisions.append((node, precision))
    bar_df = pd.DataFrame(precisions, columns=['node','precision_us']).sort_values('node')

    fig, ax = plt.subplots(figsize=(8,4))
    bars = ax.barh(
        bar_df['node'].astype(str),
        bar_df['precision_us'],
        color=COLOR_CYCLE[:len(bar_df)]
    )
    ax.set_xlabel("Ordering Precision (μs)")
    ax.set_title(f"Precision per Node ({size} nodes)")
    beautify(ax)
    for b in bars:
        ax.annotate(f"{b.get_width():.1f} μs",
                    xy=(b.get_width(), b.get_y()+b.get_height()/2),
                    xytext=(3,0), textcoords='offset points',
                    va='center', fontsize=9)
    fig.tight_layout()
    fig.savefig(f"plots/ordering_precision_bar_{size}.pdf", bbox_inches='tight')
    plt.close(fig)

def plot_ordering_precision_hist(df, size):
    df_pub = df[df['event']=='message_published']
    df_dep = df_pub[df_pub['dependencies'].apply(lambda x: isinstance(x, list) and len(x)>0)]
    if df_dep.empty:
        return
    df_dep['seq'] = pd.to_numeric(df_dep['seq'], errors='coerce')
    df_dep = df_dep.sort_values(['node','seq'])
    diffs = df_dep.groupby('node')['timestamp'].diff().dt.total_seconds().dropna()
    diffs = diffs[diffs>0]
    if diffs.empty:
        return

    mn, md, m = diffs.min(), diffs.median(), diffs.mean()
    fig, ax = plt.subplots(figsize=(8,4))
    ax.hist(diffs, bins=40, edgecolor='black')
    ax.axvline(mn, linestyle='--', label=f"min={mn:.6f}s")
    ax.axvline(md, linestyle='-.', label=f"median={md:.6f}s")
    ax.axvline(m, linestyle=':', label=f"mean={m:.6f}s")
    ax.set_xlabel("Δ Time between msgs (s)")
    ax.set_ylabel("Frequency")
    ax.set_title(f"Precision Histogram ({size} nodes)")
    ax.legend(fontsize=8)
    beautify(ax)
    fig.tight_layout()
    fig.savefig(f"plots/ordering_precision_hist_{size}.pdf", bbox_inches='tight')
    plt.close(fig)

def plot_ordering_consistency(df, size):
    df_rec = df[df['event']=='message_received']
    if df_rec.empty:
        return
    df_rec['seq'] = pd.to_numeric(df_rec['seq'], errors='coerce')
    df_rec['ordering_key'] = pd.to_numeric(df_rec['ordering_key'], errors='coerce')

    scores = []
    for node, grp in df_rec.groupby('node'):
        grp = grp.sort_values(['seq','ordering_key'])
        seqs = grp['seq'].values
        keys = grp['ordering_key'].values
        if len(seqs)>=2 and not np.isnan(keys).all():
            tau, _ = kendalltau(seqs, keys)
            scores.append((node, (tau+1)/2*100))
        else:
            scores.append((node, np.nan))
    cons_df = pd.DataFrame(scores, columns=['node','consistency']).sort_values('node')

    fig, ax = plt.subplots(figsize=(8,4))
    bars = ax.barh(
        cons_df['node'].astype(str),
        cons_df['consistency'],
        color=COLOR_CYCLE[:len(cons_df)]
    )
    ax.set_xlabel("Ordering Consistency (%)")
    ax.set_title(f"Consistency per Node ({size} nodes)")
    beautify(ax)
    for b in bars:
        val = b.get_width()
        ax.annotate(f"{val:.1f}%",
                    xy=(val, b.get_y()+b.get_height()/2),
                    xytext=(3,0), textcoords='offset points',
                    va='center', fontsize=9)
    fig.tight_layout()
    fig.savefig(f"plots/ordering_consistency_{size}.pdf", bbox_inches='tight')
    plt.close(fig)

# ─── Main ─────────────────────────────────────────────────────────────────────
def main():
    parser = argparse.ArgumentParser(
        description="Analyze logs and generate per-node + cluster plots"
    )
    parser.add_argument('logdir', help='directory with experiment_results_<N>.log files')
    args = parser.parse_args()

    os.makedirs('plots', exist_ok=True)

    # gather and sort log files by cluster size
    files = []
    for f in glob.glob(os.path.join(args.logdir, 'experiment_results_*.log')):
        m = re.search(r'experiment_results_(\d+)\.log$', f)
        if m:
            files.append((int(m.group(1)), f))
    files.sort(key=lambda x: x[0])

    throughput_summary  = []
    precision_summary   = []
    consistency_summary = []

    for size, logfile in files:
        print(f"Processing size={size} -> {logfile}")
        recs = load_logs(logfile)
        if not recs:
            print("  no valid JSON, skipping.")
            continue
        df = prepare_dataframe(recs)

        # ensure numeric columns
        df['ordering_key'] = pd.to_numeric(df.get('ordering_key', np.nan), errors='coerce')
        df['seq']          = pd.to_numeric(df.get('seq',          np.nan), errors='coerce')

        # per-node plots
        plot_aggregated_publish_rate(df, size)
        plot_ordering_precision_bar(df, size)
        plot_ordering_precision_hist(df, size)
        plot_ordering_consistency(df, size)

        # cluster-level summaries
        # throughput
        exp = df[df['event']=='experiment_summary']
        if not exp.empty:
            last = exp.sort_values('timestamp').groupby('node', as_index=False).last()
            total = last['messages_published'].astype(float).sum()
            dur   = last['run_duration_sec'].astype(float).max()
            throughput_summary.append((size, total/dur))
        # precision
        df_pub = df[df['event']=='message_published']
        deps   = df_pub[df_pub['dependencies'].apply(lambda x: isinstance(x,list) and len(x)>0)]
        if not deps.empty:
            diffs = deps.sort_values(['node','seq','timestamp'])\
                        .groupby('node')['timestamp']\
                        .diff().dt.total_seconds().dropna()
            diffs = diffs[diffs>0]
            if not diffs.empty:
                precision_summary.append((size, diffs.min()*1e6,
                                             diffs.median()*1e6,
                                             diffs.max()*1e6))
        # consistency
        df_rec = df[df['event']=='message_received']
        taus = []
        for node, grp in df_rec.groupby('node'):
            seqs = grp['seq'].values
            keys = grp['ordering_key'].values
            mask = (~np.isnan(seqs)) & (~np.isnan(keys))
            if mask.sum()>=2:
                tau, _ = kendalltau(seqs[mask], keys[mask])
                taus.append((tau+1)/2*100)
        if taus:
            consistency_summary.append((size, np.nanmean(taus)))

    # sort
    throughput_summary .sort(key=lambda x: x[0])
    precision_summary  .sort(key=lambda x: x[0])
    consistency_summary.sort(key=lambda x: x[0])

    # cluster plots
    # throughput vs size
    if throughput_summary:
        sizes, rates = zip(*throughput_summary)
        fig, ax = plt.subplots(figsize=(6,4))
        ax.plot(sizes, rates, marker='o', color=COLOR_CYCLE[0])
        ax.set_xlabel("Cluster Size")
        ax.set_ylabel("Throughput (msg/sec)")
        ax.set_title("Total Publish Throughput vs Cluster Size")
        beautify(ax)
        for x,y in zip(sizes, rates):
            ax.annotate(f"{y:.1f}", (x,y), textcoords='offset points', xytext=(0,3), ha='center')
        fig.tight_layout()
        fig.savefig('plots/total_throughput_by_size.pdf', bbox_inches='tight')
        plt.close(fig)

    # precision vs size
    if precision_summary:
        sizes, mins, meds, maxs = zip(*precision_summary)
        lower = np.array(meds)-np.array(mins)
        upper = np.array(maxs)-np.array(meds)
        fig, ax = plt.subplots(figsize=(6,4))
        ax.errorbar(sizes, meds, yerr=[lower,upper], fmt='o', capsize=5, color=COLOR_CYCLE[1])
        ax.set_xlabel("Cluster Size")
        ax.set_ylabel("Precision (μs)")
        ax.set_title("Ordering Precision vs Cluster Size")
        beautify(ax)
        fig.tight_layout()
        fig.savefig('plots/precision_vs_size.pdf', bbox_inches='tight')
        plt.close(fig)

    # consistency vs size
    if consistency_summary:
        sizes, vals = zip(*consistency_summary)
        fig, ax = plt.subplots(figsize=(6,4))
        ax.plot(sizes, vals, marker='s', linestyle='--', color=COLOR_CYCLE[2])
        ax.set_xlabel("Cluster Size")
        ax.set_ylabel("Mean Consistency (%)")
        ax.set_title("Ordering Consistency vs Cluster Size")
        beautify(ax)
        for x,y in zip(sizes, vals):
            ax.annotate(f"{y:.1f}%", (x,y), textcoords='offset points', xytext=(0,3), ha='center')
        fig.tight_layout()
        fig.savefig('plots/consistency_vs_size_per_node.pdf', bbox_inches='tight')
        plt.close(fig)

    print("Analysis complete. All PDFs saved under plots/")

if __name__ == '__main__':
    main()
