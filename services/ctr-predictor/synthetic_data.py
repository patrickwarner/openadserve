"""
Simplified Synthetic Data Generator for CTR Training
"""

import random
from datetime import datetime, timedelta
from typing import List, Tuple

import clickhouse_driver

class SyntheticDataGenerator:
    def __init__(self, clickhouse_host="localhost", clickhouse_port=9000, clickhouse_db="default"):
        self.client = clickhouse_driver.Client(
            host=clickhouse_host,
            port=clickhouse_port,
            database=clickhouse_db
        )
        
        # Realistic CTR patterns with device and country variations
        self.base_ctr = {
            ("mobile", "US"): 0.045,   # 4.5% US mobile
            ("mobile", "UK"): 0.038,   # 3.8% UK mobile  
            ("mobile", "CA"): 0.042,   # 4.2% CA mobile
            ("desktop", "US"): 0.022,  # 2.2% US desktop
            ("desktop", "UK"): 0.019,  # 1.9% UK desktop
            ("desktop", "CA"): 0.021,  # 2.1% CA desktop
        }
        
        self.countries = ["US", "UK", "CA"]
        self.devices = ["mobile", "desktop"]
        self.publishers = [1, 2, 3]  # Publisher IDs

    def generate_events(self, days: int = 7, impressions_per_day: int = 1000) -> List[Tuple]:
        """Generate simple synthetic ad events with basic mobile/desktop CTR patterns"""
        events = []
        end_time = datetime.now()
        start_time = end_time - timedelta(days=days)
        
        line_items = [100001, 100002, 100003]  # Simple CPC line items
        
        for day in range(days):
            day_start = start_time + timedelta(days=day)
            
            for _ in range(impressions_per_day):
                # Random timestamp during the day
                timestamp = day_start + timedelta(
                    hours=random.randint(0, 23),
                    minutes=random.randint(0, 59),
                    seconds=random.randint(0, 59)
                )
                
                # Generate contextual data
                device_type = random.choice(self.devices)
                country = random.choice(self.countries) 
                publisher_id = random.choice(self.publishers)
                line_item_id = random.choice(line_items)
                
                # Generate impression with full context
                request_id = f"req_{random.randint(10000, 99999)}"
                events.append((
                    timestamp, "impression", request_id, "imp1", 
                    line_item_id, line_item_id, 0.0, device_type, country, publisher_id
                ))
                
                # Context-aware click generation
                context_key = (device_type, country)
                base_ctr = self.base_ctr.get(context_key, 0.02)  # Default 2% CTR
                if random.random() < base_ctr:
                    events.append((
                        timestamp + timedelta(seconds=random.randint(1, 30)), 
                        "click", request_id, "imp1", 
                        line_item_id, line_item_id, 0.0, device_type, country, publisher_id
                    ))
        
        return events

    def insert_events(self, events: List[Tuple]):
        """Insert events into ClickHouse"""
        if not events:
            return
            
        query = """
        INSERT INTO events (timestamp, event_type, request_id, imp_id, creative_id, line_item_id, cost, device_type, country, publisher_id)
        VALUES
        """
        
        self.client.execute(query, events)

    def generate_synthetic_data(self, days: int = 7, impressions_per_day: int = 1000):
        """Generate and insert synthetic data"""
        print(f"Generating {days} days of synthetic data...")
        events = self.generate_events(days, impressions_per_day)
        
        print(f"Inserting {len(events)} events into ClickHouse...")
        self.insert_events(events)
        
        print("✅ Synthetic data generation complete!")
        
        # Show summary
        impressions = len([e for e in events if e[1] == "impression"])
        clicks = len([e for e in events if e[1] == "click"])
        ctr = (clicks / impressions * 100) if impressions > 0 else 0
        
        print(f"📊 Summary: {impressions} impressions, {clicks} clicks, {ctr:.1f}% CTR")

    def analyze_generated_data(self):
        """Analyze the generated synthetic data"""
        try:
            # Query event counts by type
            result = self.client.execute("""
                SELECT event_type, count(*) as count
                FROM events 
                GROUP BY event_type
                ORDER BY event_type
            """)
            
            print("\n📈 Generated Data Analysis:")
            for event_type, count in result:
                print(f"  {event_type}: {count:,} events")
                
            # Query CTR by device and country
            result = self.client.execute("""
                SELECT 
                    device_type,
                    country,
                    countIf(event_type = 'impression') as impressions,
                    countIf(event_type = 'click') as clicks,
                    clicks / impressions * 100 as ctr
                FROM events 
                WHERE device_type IS NOT NULL AND country IS NOT NULL
                GROUP BY device_type, country
                ORDER BY device_type, country
            """)
            
            print("\n🎯 CTR by Device & Country:")
            for device, country, imps, clicks, ctr in result:
                print(f"  {device}/{country}: {ctr:.1f}% CTR ({clicks:,} clicks / {imps:,} impressions)")
                
        except Exception as e:
            print(f"Error analyzing data: {e}")