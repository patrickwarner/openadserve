#!/usr/bin/env python3
"""
Simple integration test for the forecasting API endpoint.
This script tests the /forecast endpoint by sending a POST request.
"""

import json
import requests
import sys
from datetime import datetime, timedelta

def test_forecast_api():
    url = "http://localhost:8787/forecast"

    # Create a forecast request
    start_date = datetime.now()
    end_date = start_date + timedelta(days=7)

    request_data = {
        "start_date": start_date.isoformat() + "Z",
        "end_date": end_date.isoformat() + "Z",
        "budget_type": "cpm",
        "budget": 1000.0,
        "cpm": 5.0,
        "publisher_id": 1,
        "priority": 2,
        "device_types": ["mobile", "desktop"],
        "key_values": {
            "category": "sports"
        }
    }

    try:
        print("Testing forecast endpoint...")
        print(f"Request: {json.dumps(request_data, indent=2)}")

        response = requests.post(url, json=request_data, timeout=10)

        print(f"Status Code: {response.status_code}")

        if response.status_code == 200:
            data = response.json()
            print("✅ Success! Response:")
            print(json.dumps(data, indent=2))

            # Validate response structure
            required_fields = [
                "total_opportunities", "available_impressions",
                "estimated_impressions", "fill_rate", "conflicts"
            ]

            for field in required_fields:
                if field not in data:
                    print(f"❌ Missing field: {field}")
                    return False

            print("✅ Response has all required fields")
            return True

        elif response.status_code == 500:
            print("❌ Server error (likely no ClickHouse data)")
            print(f"Response: {response.text}")
            return False
        else:
            print(f"❌ Unexpected status code: {response.status_code}")
            print(f"Response: {response.text}")
            return False

    except requests.exceptions.ConnectionError:
        print("❌ Could not connect to server. Is it running on :8080?")
        return False
    except Exception as e:
        print(f"❌ Error: {e}")
        return False

if __name__ == "__main__":
    success = test_forecast_api()
    sys.exit(0 if success else 1)
