#!/usr/bin/env python3
"""
Pacing Strategy Analysis Tool
Analyzes and visualizes results from pacing strategy tests
"""

import subprocess
import json
import matplotlib.pyplot as plt
import pandas as pd
import yaml
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Any
import sys

class PacingAnalyzer:
    """Analyze pacing strategy test results"""
    
    def __init__(self, config_path: str = "config.yaml"):
        self.config_path = config_path
        self.config = self._load_config()
        self.strategy_map = {
            999001: 'ASAP Pacing',  # Now using actual campaign IDs
            999002: 'Even Pacing', 
            999003: 'PID Pacing'
        }
        self.colors = {
            'ASAP Pacing': '#ff4444', 
            'Even Pacing': '#44aa44', 
            'PID Pacing': '#4444ff'
        }
        
    def _load_config(self) -> Dict[str, Any]:
        """Load test configuration"""
        config_file = Path(__file__).parent / self.config_path
        try:
            with open(config_file, 'r') as f:
                return yaml.safe_load(f)
        except FileNotFoundError:
            print(f"Warning: Config file {config_file} not found, using defaults")
            return {}
    
    def _get_test_time_range(self) -> tuple:
        """Find the most recent test session time range"""
        # Get the most recent event timestamp for any pacing campaign
        query = """
        SELECT MAX(timestamp) as latest_event
        FROM events 
        WHERE campaign_id IN (999001, 999002, 999003)
          AND event_type = 'impression'
        """
        
        results = self.run_clickhouse_query(query)
        if not results or not results[0]['latest_event']:
            return None, None
            
        latest_event = results[0]['latest_event']
        
        # Get configured test duration
        duration = self.config.get('traffic', {}).get('duration', '1h')
        
        # Parse duration to hours
        if duration.endswith('h'):
            hours = int(duration[:-1])
        elif duration.endswith('m'):
            hours = int(duration[:-1]) / 60
        else:
            hours = 1  # fallback
        
        # Add 30 minutes buffer to catch the full test window
        hours_with_buffer = hours + 0.5
        
        # Calculate start time by going back from the latest event
        query_range = f"""
        SELECT 
            MIN(timestamp) as start_time,
            MAX(timestamp) as end_time
        FROM events 
        WHERE campaign_id IN (999001, 999002, 999003)
          AND event_type = 'impression'
          AND timestamp >= '{latest_event}' - INTERVAL {hours_with_buffer} HOUR
        """
        
        results = self.run_clickhouse_query(query_range)
        if not results:
            return None, None
            
        return results[0]['start_time'], results[0]['end_time']
    
    def _get_time_filter(self) -> str:
        """Generate time filter SQL based on most recent test data"""
        start_time, end_time = self._get_test_time_range()
        
        if not start_time or not end_time:
            # Fallback to recent data if no test data found
            return "AND timestamp >= now() - INTERVAL 24 HOUR"
        
        return f"AND timestamp >= '{start_time}' AND timestamp <= '{end_time}'"
    
    def run_clickhouse_query(self, query: str) -> List[Dict]:
        """Execute ClickHouse query and return results"""
        cmd = [
            "docker", "compose", "exec", "-T", "clickhouse", 
            "clickhouse-client", "--format", "JSONEachRow", "--query", query
        ]
        
        try:
            result = subprocess.run(cmd, capture_output=True, text=True, check=True)
            lines = result.stdout.strip().split('\n')
            return [json.loads(line) for line in lines if line.strip()]
        except subprocess.CalledProcessError as e:
            print(f"Error running ClickHouse query: {e}")
            print(f"Query: {query}")
            print(f"stderr: {e.stderr}")
            return []
    
    def get_test_summary(self) -> Dict[str, Any]:
        """Get overall test results summary"""
        # Get time filter based on config duration
        duration_filter = self._get_time_filter()
        
        query = f"""
        SELECT
          campaign_id,
          COUNT(*) as total_impressions,
          min(timestamp) as start_time,
          max(timestamp) as end_time,
          COUNT(DISTINCT toStartOfMinute(timestamp)) as active_minutes
        FROM events
        WHERE campaign_id IN (999001, 999002, 999003)
          AND event_type = 'impression'
          {duration_filter}
        GROUP BY campaign_id
        ORDER BY campaign_id
        """
        
        results = self.run_clickhouse_query(query)
        
        summary = {}
        for row in results:
            strategy = self.strategy_map.get(row['campaign_id'], f"Campaign {row['campaign_id']}")
            summary[strategy] = {
                'campaign_id': row['campaign_id'],
                'total_impressions': int(row['total_impressions']),
                'start_time': row['start_time'],
                'end_time': row['end_time'],
                'active_minutes': int(row['active_minutes'])
            }
        
        return summary
    
    def get_time_series_data(self, interval: str = "5min") -> pd.DataFrame:
        """Get impression data over time"""
        interval_func = {
            "1min": "toStartOfMinute",
            "2min": "toStartOfInterval(timestamp, INTERVAL 2 MINUTE)",
            "5min": "toStartOfInterval(timestamp, INTERVAL 5 MINUTE)", 
            "15min": "toStartOfInterval(timestamp, INTERVAL 15 MINUTE)"
        }
        
        time_func = interval_func.get(interval, "toStartOfMinute")
        duration_filter = self._get_time_filter()
        
        query = f"""
        SELECT
          {time_func} as time,
          campaign_id,
          COUNT(*) as impressions
        FROM events
        WHERE campaign_id IN (999001, 999002, 999003)
          AND event_type = 'impression'
          {duration_filter}
        GROUP BY time, campaign_id
        ORDER BY time, campaign_id
        """
        
        results = self.run_clickhouse_query(query)
        
        if not results:
            return pd.DataFrame()
        
        df = pd.DataFrame(results)
        df['time'] = pd.to_datetime(df['time'])
        df['strategy'] = df['campaign_id'].map(self.strategy_map)
        df['impressions'] = df['impressions'].astype(int)
        
        return df
    
    def create_visualization(self, output_file: str = None) -> str:
        """Create comprehensive visualization of pacing results"""
        
        # Get data
        summary = self.get_test_summary()
        df_5min = self.get_time_series_data("5min")
        
        if df_5min.empty:
            print("No time series data available for visualization")
            return None
        
        # Create visualization
        plt.style.use('default')
        fig, ((ax1, ax2), (ax3, ax4)) = plt.subplots(2, 2, figsize=(16, 12))
        
        # 1. Impressions per 5-minute interval
        for strategy in ['ASAP Pacing', 'Even Pacing', 'PID Pacing']:
            strategy_data = df_5min[df_5min['strategy'] == strategy].sort_values('time')
            if not strategy_data.empty:
                ax1.plot(strategy_data['time'], strategy_data['impressions'], 
                        marker='o', label=strategy, linewidth=2, markersize=4,
                        color=self.colors[strategy])
        
        ax1.set_title('Impressions per 5-Minute Interval', fontsize=14, fontweight='bold')
        ax1.set_xlabel('Time')
        ax1.set_ylabel('Impressions per 5-min')
        ax1.legend()
        ax1.grid(True, alpha=0.3)
        ax1.tick_params(axis='x', rotation=45)
        
        # 2. Cumulative impressions
        for strategy in ['ASAP Pacing', 'Even Pacing', 'PID Pacing']:
            strategy_data = df_5min[df_5min['strategy'] == strategy].sort_values('time')
            if not strategy_data.empty:
                strategy_data = strategy_data.copy()
                strategy_data['cumulative'] = strategy_data['impressions'].cumsum()
                ax2.plot(strategy_data['time'], strategy_data['cumulative'], 
                        marker='o', label=strategy, linewidth=2, markersize=4,
                        color=self.colors[strategy])
        
        ax2.set_title('Cumulative Impressions', fontsize=14, fontweight='bold')
        ax2.set_xlabel('Time')
        ax2.set_ylabel('Total Impressions')
        ax2.legend()
        ax2.grid(True, alpha=0.3)
        ax2.tick_params(axis='x', rotation=45)
        
        # 3. Total impressions bar chart
        strategies = list(summary.keys())
        totals = [summary[s]['total_impressions'] for s in strategies]
        colors = [self.colors[s] for s in strategies]
        
        bars = ax3.bar(strategies, totals, color=colors, alpha=0.7, edgecolor='black')
        ax3.set_title('Total Impressions by Strategy', fontsize=14, fontweight='bold')
        ax3.set_ylabel('Total Impressions')
        
        # Add value labels on bars
        for bar, total in zip(bars, totals):
            height = bar.get_height()
            ax3.text(bar.get_x() + bar.get_width()/2., height + height*0.01,
                    f'{total:,}', ha='center', va='bottom', fontweight='bold')
        
        # 4. Performance metrics table
        ax4.axis('tight')
        ax4.axis('off')
        
        table_data = []
        for strategy, data in summary.items():
            duration_hours = data['active_minutes'] / 60
            rate_per_hour = data['total_impressions'] / duration_hours if duration_hours > 0 else 0
            table_data.append([
                strategy,
                f"{data['total_impressions']:,}",
                f"{data['active_minutes']} min",
                f"{rate_per_hour:.0f}/hr"
            ])
        
        table = ax4.table(cellText=table_data,
                         colLabels=['Strategy', 'Total Impressions', 'Duration', 'Rate/Hour'],
                         cellLoc='center',
                         loc='center')
        table.auto_set_font_size(False)
        table.set_fontsize(10)
        table.scale(1.2, 1.5)
        
        # Color code the table rows
        for i, strategy in enumerate(strategies):
            table[(i+1, 0)].set_facecolor(self.colors[strategy])
            table[(i+1, 0)].set_text_props(weight='bold', color='white')
        
        ax4.set_title('Performance Summary', fontsize=14, fontweight='bold')
        
        plt.tight_layout(pad=3.0)
        
        # Save chart
        if not output_file:
            output_file = self.config.get('analysis', {}).get('output', {}).get('chart_file', 'pacing_analysis.png')
        
        output_path = Path(__file__).parent / output_file
        plt.savefig(output_path, dpi=150, bbox_inches='tight')
        print(f"📊 Visualization saved to: {output_path}")
        
        return str(output_path)
    
    def generate_report(self) -> str:
        """Generate markdown report of test results"""
        summary = self.get_test_summary()
        
        if not summary:
            return "No test data found. Please run the pacing test first."
        
        # Calculate winner and performance metrics
        totals = {strategy: data['total_impressions'] for strategy, data in summary.items()}
        winner = max(totals, key=totals.get)
        total_spread = max(totals.values()) - min(totals.values())
        
        report = f"""# Pacing Strategy Test Results
        
## Executive Summary

**Test Date:** {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
**Winner:** {winner} 🏆
**Performance Spread:** {total_spread} impressions

## Results by Strategy

"""
        
        for i, (strategy, data) in enumerate(sorted(summary.items(), key=lambda x: x[1]['total_impressions'], reverse=True)):
            place = ['🥇', '🥈', '🥉'][i] if i < 3 else f"{i+1}."
            duration_hours = data['active_minutes'] / 60
            rate_per_hour = data['total_impressions'] / duration_hours if duration_hours > 0 else 0
            
            report += f"""### {place} {strategy}
- **Total Impressions:** {data['total_impressions']:,}
- **Duration:** {data['active_minutes']} minutes ({duration_hours:.1f} hours)
- **Average Rate:** {rate_per_hour:.0f} impressions/hour
- **Time Range:** {data['start_time']} to {data['end_time']}

"""
        
        # Add analysis
        report += f"""## Analysis

### Key Insights
- **Efficiency Leader:** {winner} delivered {totals[winner]:,} impressions
- **Performance Variance:** {total_spread} impression spread across strategies
- **Test Duration:** {max(data['active_minutes'] for data in summary.values())} minutes

### Strategy Behavior
- **ASAP Pacing:** {"✅ Performed as expected" if "ASAP" in winner else "⚠️ Underperformed - may indicate traffic constraints"}
- **Even Pacing:** {"✅ Delivered steadily" if totals.get('Even Pacing', 0) > 0 else "❌ No data"}  
- **PID Pacing:** {"✅ Optimized delivery effectively" if "PID" in winner else "⚠️ Optimization had limited impact"}

### Recommendations
{"- Consider using " + winner + " for similar traffic patterns" if total_spread > 100 else "- All strategies performed similarly - choose based on business requirements"}

---
*Generated by Ad Server Integration Test Framework*
"""
        
        return report
    
    def print_summary(self):
        """Print a quick summary to console"""
        summary = self.get_test_summary()
        
        if not summary:
            print("❌ No test data found")
            return
        
        print("\n" + "="*50)
        print("  PACING STRATEGY TEST RESULTS")
        print("="*50)
        
        for i, (strategy, data) in enumerate(sorted(summary.items(), key=lambda x: x[1]['total_impressions'], reverse=True)):
            place = ['🥇', '🥈', '🥉'][i] if i < 3 else f"{i+1}."
            print(f"{place} {strategy}: {data['total_impressions']:,} impressions")
        
        total_impressions = sum(data['total_impressions'] for data in summary.values())
        print(f"\n📊 Total Impressions: {total_impressions:,}")
        print(f"⏱️  Test Duration: {max(data['active_minutes'] for data in summary.values())} minutes")
        print("="*50)

if __name__ == "__main__":
    analyzer = PacingAnalyzer()
    
    # Print summary
    analyzer.print_summary()
    
    # Generate visualization
    chart_path = analyzer.create_visualization()
    
    # Generate report
    report = analyzer.generate_report()
    report_file = Path(__file__).parent / "results.md"
    with open(report_file, 'w') as f:
        f.write(report)
    
    print(f"📋 Report saved to: {report_file}")
    
    if chart_path:
        print(f"📊 Chart saved to: {chart_path}")
        print("\n💡 Open the chart file to see detailed visualizations!")
    
    print("\n✅ Analysis complete!")