# CTR Optimization Integration Test

This integration test validates the CTR optimization feature by comparing ad selection behavior with and without optimization enabled.

## Test Scenarios

### 1. Without CTR Optimization
- Tests standard ad selection behavior using base eCPM values
- Ensures consistent selection patterns based on configured line item priorities

### 2. With CTR Optimization
- Tests enhanced ad selection using CTR prediction service
- Validates that CPC line items receive appropriate boost multipliers
- Demonstrates improved targeting for high-CTR contexts

### 3. Comparison Test
- Compares mobile vs desktop selection patterns
- Shows how CTR optimization affects line item ranking
- Validates that mobile users see more CPC ads due to higher predicted CTR

## Test Data

The test creates the following line items:

1. **CPM Campaign** (ID: 1)
   - Base eCPM: $5.00
   - Budget Type: CPM
   - Priority: Medium

2. **CPC Campaign** (ID: 2)
   - Base eCPM: $3.00
   - Budget Type: CPC
   - Priority: Medium
   - **Key Test Subject**: Gets CTR-based boost

3. **Another CPM Campaign** (ID: 3)
   - Base eCPM: $4.00
   - Budget Type: CPM
   - Priority: Medium

## Mock CTR Service

The test includes a mock CTR prediction service that returns:

- **Mobile**: 3% CTR, 1.5x boost multiplier
- **Desktop**: 1.5% CTR, 1.1x boost multiplier  
- **Tablet**: 2.5% CTR, 1.3x boost multiplier

This simulates realistic CTR differences across device types.

## Expected Behavior

### Without Optimization
Line items ranked by base eCPM:
1. CPM Campaign ($5.00)
2. Another CPM Campaign ($4.00)
3. CPC Campaign ($3.00)

### With Optimization (Mobile)
Line items ranked by optimized eCPM:
1. CPM Campaign ($5.00)
2. **CPC Campaign ($3.00 × 1.5 = $4.50)**
3. Another CPM Campaign ($4.00)

The CPC campaign should win more auctions on mobile due to the CTR boost.

## Running the Test

```bash
# Run the CTR optimization integration test
go test -v ./tests/integration/ctr_optimization/

# Run with environment variables
CTR_OPTIMIZATION_ENABLED=true go test -v ./tests/integration/ctr_optimization/
```

## Environment Variables

- `CTR_OPTIMIZATION_ENABLED`: Enable/disable CTR optimization (default: false)
- `CTR_PREDICTOR_URL`: URL of the CTR prediction service
- `CTR_PREDICTOR_TIMEOUT`: Timeout for CTR prediction requests (default: 100ms)

## Key Insights

This test demonstrates:

1. **Feature Toggle**: CTR optimization can be enabled/disabled without code changes
2. **Graceful Degradation**: System works normally when CTR service is unavailable
3. **Performance Impact**: CTR predictions are cached and have short timeouts
4. **Business Value**: Higher-CTR contexts receive better-performing ads