# Pacing Strategy Integration Test

Tests the three ad server pacing strategies (ASAP, Even, PID) under controlled traffic to compare their delivery patterns and performance. This test suite validates the dual-counter pacing system that separates serve counting (for pacing decisions) from impression counting (for billing accuracy).

## Usage

```bash
# Run test (keeps result files)
./run_test.sh

# Run with cleanup
./run_test.sh --cleanup-all

# Run without analysis
./run_test.sh --skip-analysis
```

## Output

- `pacing_analysis.png` - Charts showing delivery patterns over time
- `results.md` - Performance analysis and insights
- Console output with live progress

## Expected Behavior

All strategies use the dual-counter system that prevents over-delivery by using immediate serve counts for pacing decisions while tracking impression pixels for billing accuracy.

- **ASAP**: Front-loads delivery, fast initial ramp with hard cap enforcement
- **Even**: Steady linear delivery rate with smooth distribution  
- **PID**: Dynamic adjustments with hard safety checks and real-time feedback

## Configuration

Edit `config.yaml` to change:
- Test duration and traffic volume
- Daily impression caps
- Analysis parameters
- Traffic surge and jitter settings

## Troubleshooting

```bash
# Install Python dependencies
pip install matplotlib pandas pyyaml

# Check if ad server is running
curl http://localhost:8787/metrics

# See all options
./run_test.sh --help
```