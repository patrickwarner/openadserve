#!/usr/bin/env python3
"""
Reusable traffic generator for ad server integration tests.
Wrapper around the Go traffic simulator with Python configuration.
"""

import subprocess
import json
import yaml
import time
from pathlib import Path
from typing import Dict, Any, Optional


class TrafficGenerator:
    """Generate realistic traffic for ad server testing"""

    def __init__(self, config_path: Optional[str] = None):
        self.config = self._load_config(config_path) if config_path else {}
        self.project_root = Path(__file__).resolve().parent.parent.parent.parent
        self.simulator_path = self.project_root / "tools" / "traffic_simulator"

    def _load_config(self, config_path: str) -> Dict[str, Any]:
        """Load configuration from YAML file"""
        with open(config_path, "r") as f:
            return yaml.safe_load(f)

    def run_simulation(
        self,
        duration: str = "1h",
        requests: int = 10000,
        users: int = 100,
        rate: int = 25,
        concurrency: int = 10,
        api_key: str = "demo123",
        publisher_id: int = 1,
        placements: str = "header,sidebar",
        click_rate: float = 0.05,
        label: str = "test",
        server: str = "http://localhost:8787",
        surge_interval: str = "0s",
        surge_duration: str = "0s",
        surge_multiplier: float = 2.0,
        jitter: float = 0.0,
        flush_redis: bool = True,
    ) -> subprocess.CompletedProcess:
        """
        Run traffic simulation with specified parameters.
        Additional arguments allow simulation of surge traffic and jitter
        to mimic real-world fluctuations.

        Returns:
            CompletedProcess object with simulation results
        """

        simulator_relative_path = "tools/traffic_simulator/main.go"

        cmd = [
            "go",
            "run",
            simulator_relative_path,
            f"-server={server}",
            f"-users={users}",
            f"-requests={requests}",
            f"-duration={duration}",
            f"-rate={rate}",
            f"-concurrency={concurrency}",
            f"-placements={placements}",
            f"-api-key={api_key}",
            f"-publisher-id={publisher_id}",
            f"-click-rate={click_rate}",
            f"-label={label}",
            f"-surge-interval={surge_interval}",
            f"-surge-duration={surge_duration}",
            f"-surge-multiplier={surge_multiplier}",
            f"-jitter={jitter}",
            "-stats",
        ]

        if flush_redis:
            # Clear Redis before starting
            self._flush_redis()

        print(f"Running traffic simulation: {label}")
        print(f"Duration: {duration}, Requests: {requests}, Rate: {rate}/sec")
        print(f"Command: {' '.join(cmd)}")

        try:
            result = subprocess.run(
                cmd, capture_output=True, text=True, timeout=7200, cwd=self.project_root
            )  # 2 hour timeout
            return result
        except subprocess.TimeoutExpired:
            print("Traffic simulation timed out after 2 hours")
            raise
        except subprocess.CalledProcessError as e:
            print(f"Traffic simulation failed: {e}")
            print(f"stdout: {e.stdout}")
            print(f"stderr: {e.stderr}")
            raise

    def _flush_redis(self):
        """Clear Redis cache before test"""
        try:
            subprocess.run(
                ["docker", "compose", "exec", "-T", "redis", "redis-cli", "FLUSHALL"],
                check=True,
                capture_output=True,
            )
            print("Redis cache cleared")
        except subprocess.CalledProcessError as e:
            print(f"Warning: Could not clear Redis cache: {e}")

    def run_from_config(
        self, test_name: str = "default"
    ) -> subprocess.CompletedProcess:
        """Run simulation using parameters from loaded config"""
        if not self.config:
            raise ValueError("No configuration loaded")

        traffic_config = self.config.get("traffic", {})
        data_config = self.config.get("data", {})

        return self.run_simulation(
            duration=traffic_config.get("duration", "1h"),
            requests=traffic_config.get("total_requests", 10000),
            users=traffic_config.get("users", 100),
            rate=traffic_config.get("rate_per_second", 25),
            concurrency=traffic_config.get("concurrency", 10),
            api_key=data_config.get("api_key", "demo123"),
            publisher_id=data_config.get("publisher_id", 1),
            placements=",".join([p["id"] for p in data_config.get("placements", [])]),
            click_rate=traffic_config.get("click_rate", 0.05),
            label=f"{self.config.get('test', {}).get('name', 'test')}-{test_name}",
            surge_interval=traffic_config.get("surge_interval", "0s"),
            surge_duration=traffic_config.get("surge_duration", "0s"),
            surge_multiplier=traffic_config.get("surge_multiplier", 2.0),
            jitter=traffic_config.get("jitter", 0.0),
        )

    def check_prerequisites(self) -> bool:
        """Check if ad server and dependencies are running"""
        try:
            # Check if ad server is responding
            result = subprocess.run(
                ["curl", "-s", "-f", "http://localhost:8787/metrics"],
                capture_output=True,
                timeout=5,
            )
            if result.returncode != 0:
                print("❌ Ad server not responding at http://localhost:8787")
                return False

            # Check if Docker Compose is running
            result = subprocess.run(
                ["docker", "compose", "ps", "--services", "--filter", "status=running"],
                capture_output=True,
                text=True,
            )
            running_services = set(result.stdout.strip().split("\n"))
            required_services = {"openadserve", "postgres", "redis", "clickhouse"}

            if not required_services.issubset(running_services):
                missing = required_services - running_services
                print(f"❌ Missing required services: {missing}")
                return False

            print("✅ All prerequisites met")
            return True

        except subprocess.TimeoutExpired:
            print("❌ Timeout checking ad server health")
            return False
        except Exception as e:
            print(f"❌ Error checking prerequisites: {e}")
            return False


if __name__ == "__main__":
    # Example usage
    generator = TrafficGenerator()

    if generator.check_prerequisites():
        result = generator.run_simulation(
            duration="5m", requests=1000, label="quick-test"
        )
        print(f"Simulation completed with return code: {result.returncode}")
        if result.stdout:
            print("Output:", result.stdout[-500:])  # Last 500 chars
    else:
        print("Prerequisites not met. Please start the ad server stack.")
        print("Run: docker compose up -d")
