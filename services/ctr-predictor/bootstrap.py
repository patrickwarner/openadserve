#!/usr/bin/env python3
"""
Simplified Bootstrap Script for CTR Optimization
"""

import time
import urllib.request
import urllib.parse
import json
import os

def wait_for_service(url, max_attempts=30):
    """Wait for CTR predictor service to be ready."""
    print(f"⏳ Waiting for service at {url}...")
    
    for attempt in range(1, max_attempts + 1):
        try:
            response = urllib.request.urlopen(f"{url}/health", timeout=5)
            if response.getcode() == 200:
                print("✅ Service is ready!")
                return True
        except Exception:
            pass
        
        print(f"   Attempt {attempt}/{max_attempts}...")
        time.sleep(2)
    
    print("❌ Service failed to start")
    return False

def make_request(url, data=None):
    """Make HTTP request using urllib."""
    try:
        if data:
            data = json.dumps(data).encode('utf-8')
            req = urllib.request.Request(url, data=data)
            req.add_header('Content-Type', 'application/json')
        else:
            req = urllib.request.Request(url)
        
        response = urllib.request.urlopen(req, timeout=30)
        result = response.read().decode('utf-8')
        return json.loads(result) if result else {}
    except Exception as e:
        print(f"❌ Request failed: {e}")
        return None

def main():
    """Run the simplified bootstrap process."""
    base_url = "http://localhost:8000"
    
    print("🚀 Bootstrapping CTR Optimization System...")
    print("=" * 50)
    
    # Step 1: Wait for service
    if not wait_for_service(base_url):
        exit(1)
    
    # Step 2: Generate synthetic data
    print("\n🎲 Step 1: Generating synthetic data...")
    result = make_request(f"{base_url}/generate-synthetic-data", {"days": 7, "impressions_per_day": 1000})
    if result:
        print("✅ Synthetic data generated!")
    else:
        print("❌ Failed to generate data")
        exit(1)
    
    # Step 3: Train model
    print("\n🧠 Step 2: Training CTR model...")
    result = make_request(f"{base_url}/train", {"days_back": 7, "min_impressions": 5})
    if result and 'model_accuracy' in result:
        print(f"✅ Model trained! Accuracy: {result['model_accuracy']:.3f}")
    else:
        print("❌ Failed to train model")
        exit(1)
    
    # Step 4: Test prediction
    print("\n🧪 Step 3: Testing prediction...")
    test_data = {
        "line_item_id": 100001,
        "device_type": "mobile",
        "country": "US",
        "hour_of_day": 19,
        "day_of_week": 5
    }
    result = make_request(f"{base_url}/predict", test_data)
    if result and 'boost_multiplier' in result:
        print(f"✅ Prediction works! Mobile boost: {result['boost_multiplier']:.2f}x")
    else:
        print("❌ Failed to test prediction")
        exit(1)
    
    print("\n🎉 CTR Optimization Bootstrap Complete!")
    print("\n📈 System is ready for CTR-based optimization of CPC campaigns.")

if __name__ == "__main__":
    main()