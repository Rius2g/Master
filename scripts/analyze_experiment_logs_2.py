#!/usr/bin/env python3
import argparse
import json
import sys
from datetime import datetime
import pandas as pd
import matplotlib.pyplot as plt
import numpy as np

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



def plot_avg_reception_rate(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for reception rate.")
        return
    
    # Check if processing rate data exists
    if 'avg_processing_rate' not in summary.columns:
        print("No avg_processing_rate column found in summary data.")
        return
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(8,4))
    bars = plt.bar(final["node"].astype(str),
                   final["avg_processing_rate"].astype(float),
                   color='darkgreen', width=0.8)
    plt.xlabel("Node")
    plt.ylabel("Avg Reception Rate (msg/sec)")
    plt.title("Average Message Reception Rate from Smart Contract")
    
    plt.xticks(rotation=45, ha="right")
    plt.tight_layout(pad=4)

    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.2f}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center')
    plt.savefig("avg_reception_rate.png")
    plt.close()

def plot_aggregated_processing_rate(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for processing rate.")
        return
    
    # Check if processing rate data exists
    if 'avg_processing_rate' not in summary.columns:
        print("No avg_processing_rate column found in summary data.")
        return
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(8,4))
    bars = plt.bar(final["node"].astype(str),
                   final["avg_processing_rate"].astype(float),
                   color='C2', width=0.8)
    plt.xlabel("Node")
    plt.ylabel("Avg Processing Rate (msg/sec)")
    plt.title("Smart Contract Message Processing Rate per Node")
    
    # Use log scale if values are large enough
    if final["avg_processing_rate"].max() > 10:
        plt.yscale('log')
    
    plt.xticks(rotation=45, ha="right")
    plt.tight_layout(pad=4)

    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.2f}",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center')
    plt.savefig("throughput_processing_rate.png")
    plt.close()

def plot_total_messages_received(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for total messages received.")
        return
    
    # Check if messages received data exists
    if 'messages_received' not in summary.columns:
        print("No messages_received column found in summary data.")
        return
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    plt.figure(figsize=(8,4))
    bars = plt.bar(final["node"].astype(str),
                   final["messages_received"].astype(int),
                   color='C3')
    plt.xlabel("Node")
    plt.ylabel("Total Messages Received")
    dur = get_test_duration(df)
    title = "Total Messages Received from Smart Contract"
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
    plt.savefig("throughput_messages_received.png")
    plt.close()

def plot_publish_vs_process_rate(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for publish vs process rate.")
        return
    
    # Check if required columns exist
    required_cols = ['avg_publish_rate', 'avg_processing_rate']
    if not all(col in summary.columns for col in required_cols):
        print("Missing columns for publish vs process rate analysis.")
        return
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    # Create a grouped bar chart
    plt.figure(figsize=(10,5))
    
    bar_width = 0.35
    index = np.arange(len(final))
    
    plt.bar(index, final["avg_publish_rate"], bar_width, label='Publish Rate', color='C0')
    plt.bar(index + bar_width, final["avg_processing_rate"], bar_width, label='Processing Rate', color='C2')
    
    plt.xlabel('Node')
    plt.ylabel('Messages per Second')
    plt.title('Publish Rate vs Processing Rate by Node')
    plt.xticks(index + bar_width/2, final["node"])
    plt.legend()
    
    plt.tight_layout()
    plt.savefig("publish_vs_process_rate.png")
    plt.close()

def plot_end_to_end_ratio(df):
    summary = df[df["event"] == "experiment_summary"].copy()
    if summary.empty:
        print("No experiment_summary for end-to-end ratio.")
        return
    
    # Check if end_to_end_ratio exists
    if 'end_to_end_ratio' not in summary.columns:
        print("No end_to_end_ratio column found in summary data.")
        return
    
    final = (
        summary
        .sort_values("timestamp")
        .groupby("node", as_index=False)
        .last()
    )
    
    # Convert to percentage
    final["end_to_end_ratio_pct"] = final["end_to_end_ratio"] * 100
    
    plt.figure(figsize=(8,5))
    bars = plt.bar(final["node"].astype(str),
                  final["end_to_end_ratio_pct"],
                  color='C5')
    
    plt.axhline(y=100, color='r', linestyle='--', alpha=0.7)
    
    plt.xlabel("Node")
    plt.ylabel("End-to-End Ratio (%)")
    plt.title("Percentage of Published Messages Successfully Processed")
    plt.ylim(0, max(100, final["end_to_end_ratio_pct"].max() * 1.1))
    
    for b in bars:
        h = b.get_height()
        plt.annotate(f"{h:.1f}%",
                     xy=(b.get_x()+b.get_width()/2, h),
                     xytext=(0,3), textcoords='offset points',
                     ha='center')
    
    plt.tight_layout()
    plt.savefig("end_to_end_ratio.png")
    plt.close()

def plot_processor_stats_time_series(df):
    """Plot processing rate over time from node_processing_stats events"""
    
    # Get processor stats events
    proc_stats = df[df["event"] == "node_processing_stats"].copy()
    if proc_stats.empty:
        print("No node_processing_stats events found for time series analysis.")
        return
    
    # Check if required columns exist
    required_cols = ['messages_processed', 'messages_per_sec', 'interval_msgs_per_sec']
    if not all(col in proc_stats.columns for col in required_cols):
        print(f"Missing columns for processor stats time series analysis. Available: {proc_stats.columns.tolist()}")
        return
    
    # Convert to numeric, coercing errors to NaN
    for col in required_cols:
        proc_stats[col] = pd.to_numeric(proc_stats[col], errors='coerce')
    
    # Sort by timestamp
    proc_stats = proc_stats.sort_values('timestamp')
    
    # Create figure with two subplots
    fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(12, 10), sharex=True)
    
    # Plot cumulative messages processed
    for node, group in proc_stats.groupby("node"):
        ax1.plot(group["timestamp"], 
                group["messages_processed"], 
                marker='o', 
                markersize=4,
                linewidth=2,
                alpha=0.7,
                label=f"Node {node}")
    
    ax1.set_ylabel("Total Messages Processed")
    ax1.set_title("Cumulative Messages Processed Over Time")
    ax1.legend()
    ax1.grid(True)
    
    # Plot processing rate
    for node, group in proc_stats.groupby("node"):
        ax2.plot(group["timestamp"], 
                group["interval_msgs_per_sec"], 
                marker='s', 
                markersize=4,
                linewidth=2,
                alpha=0.7,
                label=f"Node {node}")
    
    ax2.set_xlabel("Time")
    ax2.set_ylabel("Processing Rate (msg/sec)")
    ax2.set_title("Message Processing Rate Over Time")
    ax2.legend()
    ax2.grid(True)
    
    # Format x-axis
    fig.autofmt_xdate()
    
    plt.tight_layout()
    plt.savefig("processor_stats_time_series.png")
    plt.close()


def main():
    p = argparse.ArgumentParser(
        description="Analyze logs: processing rate metrics"
    )
    p.add_argument('logfile', help='path to experiment_results.log')
    args = p.parse_args()

    recs = load_logs(args.logfile)
    if not recs:
        print("No JSON records.")
        sys.exit(1)
    df = prepare_dataframe(recs)

    plot_aggregated_publish_rate(df)

    print("▶ Message Processing Metrics:")
    plot_publish_vs_process_rate(df) 
    plot_end_to_end_ratio(df)
    
    print("\nAnalysis complete. Images saved to current directory.")

if __name__ == '__main__':
    main()
