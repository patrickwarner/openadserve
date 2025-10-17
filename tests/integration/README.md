# Ad Server Integration Tests

This directory contains integration tests that demonstrate and validate key ad serving capabilities in realistic scenarios.

## Available Tests

### [Pacing Strategies](./pacing/)
Tests and compares different ad delivery pacing strategies:
- **ASAP Pacing**: Delivers impressions as quickly as possible
- **Even Pacing**: Distributes impressions evenly over time
- **PID Pacing**: Uses PID control for smooth delivery adjustments

### Future Tests
- **Frequency Capping**: Validates user-level impression limits
- **Targeting**: Tests geo, device, and custom targeting accuracy
- **Rate Limiting**: Validates traffic control mechanisms

## Quick Start

```bash
# Run pacing strategy comparison test
cd tests/integration/pacing
./run_test.sh

# View results
python3 analyze.py
```

## Test Philosophy

These integration tests serve multiple purposes:

1. **Validation**: Ensure core ad serving logic works correctly
2. **Demonstration**: Show prospective clients how features work
3. **Performance**: Measure system behavior under realistic load
4. **Documentation**: Living examples of feature capabilities

## Requirements

- Docker Compose stack running (`docker compose up`)
- Python 3 with matplotlib and pandas (for analysis)
- PostgreSQL and ClickHouse containers accessible

## Test Data

Each test creates isolated test data that doesn't interfere with production or other tests. Test data uses dedicated publisher IDs (999+) and can be safely cleaned up.