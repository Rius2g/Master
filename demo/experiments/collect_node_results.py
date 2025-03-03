#!/usr/bin/env python3
"""
Simple Experiment Results Collector

This script connects to all experiment nodes and collects their results.
It creates a consolidated output for easy analysis.
"""

import argparse
import json
import os
import sys
import requests
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime

def parse_args():
    parser = argparse.ArgumentParser(description="Collect experiment results from nodes")
    parser.add_argument("--nodes", type=str, required=True, 
                        help="Comma-separated list of node URLs (e.g., http://node1:8080,http://node2:8080)")
    parser.add_argument("--output", type=str, default="experiment_results.json", 
                        help="Output file for collected results")
    parser.add_argument("--timeout", type=int, default=30, 
                        help="Connection timeout in seconds")
    parser.add_argument("--summary-only", action="store_true", 
                        help="Only collect summary data, not full results")
    return parser.parse_args()

def collect_from_node(node_url, summary_only=False, timeout=30):
    """Collect results from a single node"""
    node_data = {
        "url": node_url,
        "status": "unknown"
    }
    
    try:
        # Check node health
        health_resp = requests.get(f"{node_url}/health", timeout=timeout)
        if health_resp.status_code != 200:
            node_data["status"] = "error"
            node_data["error"] = f"Health check failed: {health_resp.status_code}"
            return node_data
        
        node_data["health"] = health_resp.json()
        
        # Get summary or full results
        endpoint = "/summary" if summary_only else "/results"
        results_resp = requests.get(f"{node_url}{endpoint}", timeout=timeout)
        
        if results_resp.status_code != 200:
            node_data["status"] = "error"
            node_data["error"] = f"Results fetch failed: {results_resp.status_code}"
            return node_data
        
        # Add results to node data
        result_key = "summary" if summary_only else "results"
        node_data[result_key] = results_resp.json()
        node_data["status"] = "success"
        
        return node_data
        
    except Exception as e:
        node_data["status"] = "error"
        node_data["error"] = str(e)
        return node_data

def collect_all_results(nodes, summary_only=False, timeout=30):
    """Collect results from all nodes in parallel"""
    node_urls = [url.strip() for url in nodes.split(",")]
    
    print(f"Collecting {'summary' if summary_only else 'full results'} from {len(node_urls)} nodes...")
    
    results = []
    with ThreadPoolExecutor(max_workers=10) as executor:
        # Create a future for each node
        future_to_url = {
            executor.submit(collect_from_node, url, summary_only, timeout): url 
            for url in node_urls
        }
        
        # Process as they complete
        for future in future_to_url:
            url = future_to_url[future]
            try:
                result = future.result()
                results.append(result)
                status = result.get("status", "unknown")
                if status == "success":
                    print(f"✓ Successfully collected from {url}")
                else:
                    print(f"✗ Failed to collect from {url}: {result.get('error', 'Unknown error')}")
            except Exception as e:
                print(f"✗ Error collecting from {url}: {e}")
                results.append({"url": url, "status": "error", "error": str(e)})
    
    return results

def main():
    args = parse_args()
    
    # Collect results from all nodes
    node_results = collect_all_results(
        args.nodes, 
        summary_only=args.summary_only,
        timeout=args.timeout
    )
    
    # Prepare consolidated output
    output_data = {
        "collection_time": datetime.now().isoformat(),
        "nodes_total": len(node_results),
        "nodes_success": sum(1 for n in node_results if n.get("status") == "success"),
        "nodes_error": sum(1 for n in node_results if n.get("status") == "error"),
        "node_results": node_results
    }
    
    # Write output
    with open(args.output, 'w') as f:
        json.dump(output_data, f, indent=2)
    
    print(f"\nResults collected from {output_data['nodes_success']} of {output_data['nodes_total']} nodes")
    print(f"Output saved to: {args.output}")
    
    if output_data['nodes_error'] > 0:
        print(f"Warning: Failed to collect from {output_data['nodes_error']} nodes")
        return 1
    
    return 0

if __name__ == "__main__":
    sys.exit(main())
