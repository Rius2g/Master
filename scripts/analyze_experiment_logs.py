#!/usr/bin/env python3
import argparse
import json
import sys
from datetime import datetime

import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
from scipy.stats import kendalltau

plt.style.use('ggplot')
plt.rcParams.update({
    'axes.titlesize': 14,
    'axes.labelsize': 12,
    'xtick.labelsize': 10,
    'ytick.labelsize': 10,
})

def get_test_duration(df):
    start = df[df["event"] == "application_start"]["timestamp"]
    end   = df[df["event"] == "application_shutdown"]["timestamp"]
    if start.empty or end.empty:
        return None
    return (end.max() - start.min()).total_seconds()

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

# ─── throughput plots ──────────────────────────────────────────────────────────

def plot_aggregated_publish_rate(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for publish rate.")
        return
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    plt.figure(figsize=(8,4))
    bars = plt.bar(final["node"].astype(str),
                   final["avg_publish_rate"].astype(float),
                   color='C0', width=0.8)
    plt.xlabel("Node")
    plt.ylabel("Avg Publish Rate (msg/sec)")
    plt.yscale('log')
    plt.title("Aggregated Publish Rate per Node")
    
    plt.xticks(rotation=45, ha="right")  # Rotate x-axis labels
    plt.tight_layout(pad=4)

    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.2f}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center')
    plt.savefig("throughput_publish_rate.png")
    plt.close()

def plot_total_messages_published(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for total messages.")
        return
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    plt.figure(figsize=(8,4))
    bars = plt.bar(final["node"].astype(str),
                   final["messages_published"].astype(int),
                   color='C1')
    plt.xlabel("Node")
    plt.ylabel("Total Messages Published")
    dur = get_test_duration(df)
    title = "Total Messages per Node"
    if dur:
        title += f" (over {dur:.1f}s)"
    plt.title(title)
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center')
    plt.tight_layout()
    plt.savefig("throughput_total_messages.png")
    plt.close()

# ─── cost analysis plots ──────────────────────────────────────────────────────────

def plot_total_cost(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for total cost.")
        return
    
    # Check if cost data exists
    if 'total_cost_eth' not in summary.columns:
        print("No total_cost_eth column found in summary data.")
        return
    
    # Convert to numeric, coercing errors to NaN
    summary["total_cost_eth"] = pd.to_numeric(summary["total_cost_eth"], errors='coerce')
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(10,5))
    bars = plt.bar(final["node"].astype(str),
                   final["total_cost_eth"],
                   color='#2E8B57')  # Sea Green
    plt.xlabel("Node")
    plt.ylabel("Total Cost (ETH)")
    plt.title("Total Blockchain Cost per Node")
    
    # Format y-axis to show scientific notation for small values
    plt.ticklabel_format(axis='y', style='sci', scilimits=(0,0))
    
    # Add value labels on top of bars
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.8f}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center', rotation=45)
    
    plt.xticks(rotation=45, ha="right")
    plt.tight_layout()
    plt.savefig("cost_total.png")
    plt.close()

def plot_cost_per_message(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for cost per message.")
        return
    
    # Check if cost per message data exists
    if 'cost_per_message_eth' not in summary.columns:
        print("No cost_per_message_eth column found in summary data.")
        return
    
    # Convert to numeric, coercing errors to NaN
    summary["cost_per_message_eth"] = pd.to_numeric(summary["cost_per_message_eth"], errors='coerce')
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(10,5))
    bars = plt.bar(final["node"].astype(str),
                   final["cost_per_message_eth"],
                   color='#E9967A')  # Dark Salmon
    plt.xlabel("Node")
    plt.ylabel("Cost per Message (ETH)")
    plt.title("Blockchain Cost per Message by Node")
    
    # Format y-axis to show scientific notation for small values
    plt.ticklabel_format(axis='y', style='sci', scilimits=(0,0))
    
    # Add value labels on top of bars
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.10f}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center', rotation=45)
    
    plt.xticks(rotation=45, ha="right")
    plt.tight_layout()
    plt.savefig("cost_per_message.png")
    plt.close()

def plot_cost_per_byte(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for cost per byte.")
        return
    
    # Check if cost per byte data exists
    if 'cost_per_byte_eth' not in summary.columns:
        print("No cost_per_byte_eth column found in summary data.")
        return
    
    # Convert to numeric, coercing errors to NaN
    summary["cost_per_byte_eth"] = pd.to_numeric(summary["cost_per_byte_eth"], errors='coerce')
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(10,5))
    bars = plt.bar(final["node"].astype(str),
                   final["cost_per_byte_eth"],
                   color='#6495ED')  # Cornflower Blue
    plt.xlabel("Node")
    plt.ylabel("Cost per Byte (ETH)")
    plt.title("Blockchain Cost per Byte by Node")
    
    # Format y-axis to show scientific notation for small values
    plt.ticklabel_format(axis='y', style='sci', scilimits=(0,0))
    
    # Add value labels on top of bars
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.12f}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center', rotation=45)
    
    plt.xticks(rotation=45, ha="right")
    plt.tight_layout()
    plt.savefig("cost_per_byte.png")
    plt.close()

def plot_gas_usage(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for gas usage.")
        return
    
    # Check if gas usage data exists
    if 'total_gas_used' not in summary.columns:
        print("No total_gas_used column found in summary data.")
        return
    
    # Convert to numeric, coercing errors to NaN
    summary["total_gas_used"] = pd.to_numeric(summary["total_gas_used"], errors='coerce')
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(10,5))
    bars = plt.bar(final["node"].astype(str),
                   final["total_gas_used"],
                   color='#9370DB')  # Medium Purple
    plt.xlabel("Node")
    plt.ylabel("Gas Used")
    plt.title("Total Gas Usage by Node")
    
    # Add value labels on top of bars
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{int(h):,}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center', rotation=45)
    
    plt.xticks(rotation=45, ha="right")
    plt.tight_layout()
    plt.savefig("gas_usage.png")
    plt.close()

def plot_avg_gas_per_message(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for avg gas per message.")
        return
    
    # Check if avg gas per message data exists
    if 'avg_gas_per_msg' not in summary.columns:
        print("No avg_gas_per_msg column found in summary data.")
        return
    
    # Convert to numeric, coercing errors to NaN
    summary["avg_gas_per_msg"] = pd.to_numeric(summary["avg_gas_per_msg"], errors='coerce')
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(10,5))
    bars = plt.bar(final["node"].astype(str),
                   final["avg_gas_per_msg"],
                   color='#FF7F50')  # Coral
    plt.xlabel("Node")
    plt.ylabel("Average Gas per Message")
    plt.title("Average Gas Usage per Message by Node")
    
    # Add value labels on top of bars
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{int(h):,}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center', rotation=45)
    
    plt.xticks(rotation=45, ha="right")
    plt.tight_layout()
    plt.savefig("avg_gas_per_message.png")
    plt.close()

def plot_cost_vs_throughput(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for cost vs. throughput analysis.")
        return
    
    # Check if required columns exist
    required_cols = ['avg_publish_rate', 'cost_per_message_eth']
    if not all(col in summary.columns for col in required_cols):
        print("Missing columns for cost vs. throughput analysis.")
        return
    
    # Convert to numeric, coercing errors to NaN
    for col in required_cols:
        summary[col] = pd.to_numeric(summary[col], errors='coerce')
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(10,6))
    plt.scatter(final["avg_publish_rate"], 
                final["cost_per_message_eth"], 
                s=80, 
                alpha=0.7,
                c='#4169E1')  # Royal Blue
    
    # Add node labels to each point
    for i, node in enumerate(final["node"]):
        plt.annotate(node, 
                     (final["avg_publish_rate"].iloc[i], final["cost_per_message_eth"].iloc[i]),
                     xytext=(5, 5), 
                     textcoords='offset points')
    
    plt.xlabel("Throughput (messages/sec)")
    plt.ylabel("Cost per Message (ETH)")
    plt.title("Cost Efficiency vs. Throughput")
    
    # Format y-axis to show scientific notation for small values
    plt.ticklabel_format(axis='y', style='sci', scilimits=(0,0))
    
    # Add trend line
    if len(final) > 1:
        z = np.polyfit(final["avg_publish_rate"], final["cost_per_message_eth"], 1)
        p = np.poly1d(z)
        x_range = np.linspace(final["avg_publish_rate"].min(), final["avg_publish_rate"].max(), 100)
        plt.plot(x_range, p(x_range), "r--", alpha=0.7)
        
        # Add correlation coefficient
        corr = final["avg_publish_rate"].corr(final["cost_per_message_eth"])
        plt.annotate(f"Correlation: {corr:.2f}", 
                    xy=(0.05, 0.95), 
                    xycoords='axes fraction',
                    bbox=dict(boxstyle="round,pad=0.3", fc="white", ec="gray", alpha=0.8))
    
    plt.grid(True)
    plt.tight_layout()
    plt.savefig("cost_vs_throughput.png")
    plt.close()

def plot_cost_time_series(df):
    """Plot how costs evolved over time during the experiment"""
    
    # Get periodic stats that have cost data
    stats = df[df["event"] == "periodic_stats"].copy()
    if stats.empty:
        print("No periodic_stats events found for time series analysis.")
        return
    
    # Check if required columns exist
    if 'total_cost_eth' not in stats.columns:
        print("No total_cost_eth column found in stats data.")
        return
    
    # Convert to numeric, coercing errors to NaN
    stats["total_cost_eth"] = pd.to_numeric(stats["total_cost_eth"], errors='coerce')
    stats["elapsed_seconds"] = pd.to_numeric(stats["elapsed_seconds"], errors='coerce')
    
    # Plot time series for each node
    plt.figure(figsize=(12,6))
    
    for node, group in stats.groupby("node"):
        group = group.sort_values("elapsed_seconds")
        plt.plot(group["elapsed_seconds"], 
                group["total_cost_eth"], 
                marker='o', 
                markersize=4,
                linewidth=2,
                alpha=0.7,
                label=f"Node {node}")
    
    plt.xlabel("Elapsed Time (seconds)")
    plt.ylabel("Total Cost (ETH)")
    plt.title("Cumulative Cost Over Time")
    
    # Format y-axis to show scientific notation for small values
    plt.ticklabel_format(axis='y', style='sci', scilimits=(0,0))
    
    plt.legend()
    plt.grid(True)
    plt.tight_layout()
    plt.savefig("cost_time_series.png")
    plt.close()

# ─── ordering‐precision plots ──────────────────────────────────────────────────

def plot_ordering_precision_bar(df):
    df_pub = df[df['event']=='message_published'].copy()
    df_dep = df_pub[df_pub['dependencies'].apply(lambda x: isinstance(x,list) and len(x)>0)]
    if df_dep.empty:
        print("No dependent messages for precision.")
        return
    df_dep['seq'] = pd.to_numeric(df_dep['seq'], errors='coerce')
    precisions = []
    for node, grp in df_dep.groupby('node'):
        grp = grp.sort_values('seq')
        deltas = grp['timestamp'].diff().dt.total_seconds().dropna()
        pos    = deltas[deltas>0]
        precisions.append((node, pos.min()*1e6 if not pos.empty else np.nan))
    bar_df = pd.DataFrame(precisions, columns=['node','precision_us']).sort_values('node')

    plt.figure(figsize=(8,6))
    bars = plt.bar(bar_df['node'].astype(str),
                   bar_df['precision_us'],
                   color='seagreen')
    plt.xlabel('Node')
    plt.ylabel('Ordering Precision (μs)')
    plt.title('Message Ordering Precision per Node')
    plt.ylim(0, bar_df['precision_us'].max()*1.1)
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.1f} μs",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center')
    plt.tight_layout()
    plt.savefig('ordering_precision_bar.png')
    plt.close()

def plot_ordering_precision_hist(df):
    df_pub = df[df['event']=='message_published'].copy()
    df_dep = df_pub[df_pub['dependencies'].apply(lambda x: isinstance(x,list) and len(x)>0)]
    if df_dep.empty:
        print("No dependent messages for histogram.")
        return
    df_dep['seq'] = pd.to_numeric(df_dep['seq'], errors='coerce')
    df_dep.sort_values(['node','seq'], inplace=True)
    diffs = df_dep.groupby('node')['timestamp'].diff().dt.total_seconds().dropna()
    diffs = diffs[diffs>0]
    if diffs.empty:
        print("No valid time differences.")
        return

    mn, md, m = diffs.min(), diffs.median(), diffs.mean()
    plt.figure(figsize=(8,6))
    plt.hist(diffs, bins=40, color='cornflowerblue', edgecolor='black')
    plt.xlabel('Time Δ between consecutive messages (s)')
    plt.ylabel('Frequency')
    plt.title(f'Ordering Precision Histogram\nMin={mn:.6f}s, Median={md:.6f}s, Mean={m:.6f}s')
    plt.tight_layout()
    plt.savefig('ordering_precision_hist.png')
    plt.close()

# ─── ordering‐consistency (Kendall τ) ─────────────────────────────────────────

def plot_ordering_consistency(df):
    df_pub = df[df['event']=='message_published'].copy()
    df_pub['seq'] = pd.to_numeric(df_pub['seq'], errors='coerce')
    scores = []
    for node, grp in df_pub.groupby('node'):
        grp = grp.sort_values('seq')
        seqs  = grp['seq'].values
        times = grp['timestamp'].astype(np.int64) / 1e9  # to seconds
        if len(seqs) >= 2:
            tau, _ = kendalltau(seqs, times)
            score = (tau + 1) / 2 * 100
        else:
            score = np.nan
        scores.append((node, score))
    cons_df = pd.DataFrame(scores, columns=['node','consistency']).sort_values('node')

    plt.figure(figsize=(8,6))
    bars = plt.bar(cons_df['node'].astype(str),
                   cons_df['consistency'],
                   color='orchid')
    plt.xlabel('Node')
    plt.ylabel('Ordering Consistency (%)')
    plt.title('Message Ordering Consistency per Node (Kendall τ)', pad=20)
    plt.ylim(0,100)
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.1f}%",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center')
    plt.tight_layout(pad=2)
    plt.savefig('ordering_consistency.png')
    plt.close()

# ─── cost efficiency analysis ────────────────────────────────────────────────────

def calculate_cost_efficiency_metrics(df):
    """Calculate and print cost efficiency metrics"""
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary data for cost efficiency analysis.")
        return
    
    # Check if required columns exist
    required_cols = ['messages_published', 'total_cost_eth', 'total_gas_used', 'avg_publish_rate']
    if not all(col in summary.columns for col in required_cols):
        print("Missing columns for cost efficiency analysis.")
        return
    
    # Convert to numeric, coercing errors to NaN
    for col in required_cols:
        summary[col] = pd.to_numeric(summary[col], errors='coerce')
    
    # Aggregate across all nodes
    total_messages = summary['messages_published'].sum()
    total_cost_eth = summary['total_cost_eth'].sum()
    total_gas = summary['total_gas_used'].sum()
    avg_rate = summary['avg_publish_rate'].mean()
    
    # Calculate metrics
    cost_per_msg = total_cost_eth / total_messages if total_messages > 0 else np.nan
    gas_per_msg = total_gas / total_messages if total_messages > 0 else np.nan
    msgs_per_eth = total_messages / total_cost_eth if total_cost_eth > 0 else np.nan
    
    # Create summary dataframe
    metrics = pd.DataFrame({
        'Metric': [
            'Total Messages', 
            'Total Cost (ETH)', 
            'Cost per Message (ETH)', 
            'Messages per ETH',
            'Total Gas Used',
            'Gas per Message',
            'Average Throughput (msg/sec)'
        ],
        'Value': [
            f"{total_messages:,.0f}",
            f"{total_cost_eth:.8f}",
            f"{cost_per_msg:.12f}",
            f"{msgs_per_eth:,.0f}",
            f"{total_gas:,.0f}",
            f"{gas_per_msg:.2f}",
            f"{avg_rate:.2f}"
        ]
    })
    
    # Print and save metrics
    print("\n▶ Cost Efficiency Metrics:")
    print(metrics.to_string(index=False))
    
    metrics.to_csv("cost_efficiency_metrics.csv", index=False)

# ─── main ─────────────────────────────────────────────────────────────────────

def main():
    p = argparse.ArgumentParser(
        description="Analyze logs: throughput + precision + consistency + costs."
    )
    p.add_argument('logfile', help='path to experiment_results.log')
    p.add_argument('--cost-only', action='store_true', help='Only run cost analysis')
    args = p.parse_args()

    recs = load_logs(args.logfile)
    if not recs:
        print("No JSON records.")
        sys.exit(1)
    df = prepare_dataframe(recs)

    if not args.cost_only:
        print("▶ Throughput:")
        plot_aggregated_publish_rate(df)
        plot_total_messages_published(df)

        print("▶ Ordering Precision:")
        plot_ordering_precision_bar(df)
        plot_ordering_precision_hist(df)

        print("▶ Ordering Consistency:")
        plot_ordering_consistency(df)
    
    print("▶ Cost Analysis:")
    plot_total_cost(df)
    plot_cost_per_message(df)
    plot_cost_per_byte(df)
    plot_gas_usage(df)
    plot_avg_gas_per_message(df)
    plot_cost_vs_throughput(df)
    plot_cost_time_series(df)
    
    # Calculate overall cost efficiency metrics
    calculate_cost_efficiency_metrics(df)
    
    print("\nAnalysis complete. Images saved to current directory.")

if __name__ == '__main__':
    main()
