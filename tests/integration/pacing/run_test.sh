#!/bin/bash

# Pacing Strategy Integration Test Runner
# Comprehensive test of ASAP, Even, and PID pacing strategies

set -e

# Parse command line arguments
CLEANUP_BEFORE=false
CLEANUP_AFTER=false
SKIP_ANALYSIS=false
HELP=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --cleanup-before)
            CLEANUP_BEFORE=true
            shift
            ;;
        --cleanup-after)
            CLEANUP_AFTER=true
            shift
            ;;
        --cleanup-all)
            CLEANUP_BEFORE=true
            CLEANUP_AFTER=true
            shift
            ;;
        --skip-analysis)
            SKIP_ANALYSIS=true
            shift
            ;;
        -h|--help)
            HELP=true
            shift
            ;;
        *)
            echo "Unknown option $1"
            HELP=true
            shift
            ;;
    esac
done

if [ "$HELP" = true ]; then
    echo "Pacing Strategy Integration Test Runner"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --cleanup-before    Clean up any existing test data before running"
    echo "  --cleanup-after     Clean up test data after completing the test"
    echo "  --cleanup-all       Clean up before and after (equivalent to --cleanup-before --cleanup-after)"
    echo "  --skip-analysis     Skip Python analysis and visualization generation"
    echo "  -h, --help          Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                           # Basic test run"
    echo "  $0 --cleanup-all             # Clean run with full cleanup"
    echo "  $0 --cleanup-before          # Clean existing data before test"
    echo "  $0 --cleanup-after           # Clean up after test completes"
    echo ""
    exit 0
fi

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CONFIG_FILE="$SCRIPT_DIR/config.yaml"
SETUP_SQL="$SCRIPT_DIR/setup.sql"

# Cleanup function
cleanup_test_data() {
    local when=$1  # "before" or "after"

    echo -e "${BLUE}🧹 Cleaning up test data ($when test)...${NC}"

    # Clean up database records in correct order (foreign key constraints)
    echo "  📊 Cleaning events data..."
    # Clean ClickHouse events (where events are actually stored)
    docker compose exec -T clickhouse clickhouse-client --query \
        "DELETE FROM events WHERE campaign_id IN (999001, 999002, 999003)" > /dev/null 2>&1 || true
    # Clean PostgreSQL events (legacy/backup, might not exist)
    docker compose exec -T postgres psql -U postgres -d postgres -c \
        "DELETE FROM events WHERE campaign_id IN (999001, 999002, 999003);" > /dev/null 2>&1 || true

    echo "  🎨 Cleaning creatives..."
    docker compose exec -T postgres psql -U postgres -d postgres -c \
        "DELETE FROM creatives WHERE publisher_id = 999;" > /dev/null 2>&1 || true

    echo "  📋 Cleaning line items..."
    docker compose exec -T postgres psql -U postgres -d postgres -c \
        "DELETE FROM line_items WHERE publisher_id = 999;" > /dev/null 2>&1 || true

    echo "  📢 Cleaning campaigns..."
    docker compose exec -T postgres psql -U postgres -d postgres -c \
        "DELETE FROM campaigns WHERE publisher_id = 999;" > /dev/null 2>&1 || true

    echo "  📍 Cleaning placements..."
    docker compose exec -T postgres psql -U postgres -d postgres -c \
        "DELETE FROM placements WHERE publisher_id = 999;" > /dev/null 2>&1 || true

    echo "  🏢 Cleaning publishers..."
    docker compose exec -T postgres psql -U postgres -d postgres -c \
        "DELETE FROM publishers WHERE id = 999;" > /dev/null 2>&1 || true

    # Clear Redis cache
    echo "  🗄️  Clearing Redis cache..."
    docker compose exec -T redis redis-cli FLUSHALL > /dev/null 2>&1 || true

    # Clean up result files if this is after-test cleanup
    if [ "$when" = "after" ]; then
        echo "  📁 Cleaning result files..."
        rm -f "$SCRIPT_DIR/pacing_analysis.png" 2>/dev/null || true
        rm -f "$SCRIPT_DIR/results.md" 2>/dev/null || true
    fi

    echo -e "${GREEN}✅ Cleanup completed${NC}"
}

# Error handling with cleanup
cleanup_on_error() {
    echo -e "${RED}❌ Test failed with error${NC}"
    if [ "$CLEANUP_AFTER" = true ]; then
        echo -e "${YELLOW}🧹 Running cleanup due to --cleanup-after flag...${NC}"
        cleanup_test_data "error"
    fi
    exit 1
}

# Set up error handling
trap cleanup_on_error ERR

echo -e "${BLUE}"
echo "=============================================="
echo "  AD SERVER PACING STRATEGY INTEGRATION TEST"
echo "=============================================="
echo -e "${NC}"

# Check prerequisites
echo -e "${BLUE}🔍 Checking prerequisites...${NC}"

# Check if Docker Compose is running
if ! docker compose ps > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker Compose not found or not running${NC}"
    echo "Please start the ad server stack first:"
    echo "  cd $PROJECT_ROOT && docker compose up -d"
    exit 1
fi

# Check if ad server is responding
if ! curl -s -f http://localhost:8787/metrics > /dev/null 2>&1; then
    echo -e "${RED}❌ Ad server not responding${NC}"
    echo "Please ensure the ad server is running:"
    echo "  docker compose ps"
    exit 1
fi

echo -e "${GREEN}✅ Prerequisites met${NC}"

# Optional cleanup before test
if [ "$CLEANUP_BEFORE" = true ]; then
    cleanup_test_data "before"
fi

# Load test data
echo -e "${BLUE}📊 Setting up test data...${NC}"

# Read configuration values from config file
DAILY_CAP=$(grep -A 10 "^data:" "$CONFIG_FILE" | grep "daily_impression_cap:" | head -1 | sed 's/.*daily_impression_cap: *\([0-9]*\).*/\1/')
CPM=$(grep -A 10 "^data:" "$CONFIG_FILE" | grep "cpm:" | head -1 | sed 's/.*cpm: *\([0-9.]*\).*/\1/')
BUDGET_MULTIPLIER=$(grep -A 10 "^data:" "$CONFIG_FILE" | grep "budget_multiplier:" | head -1 | sed 's/.*budget_multiplier: *\([0-9.]*\).*/\1/')

# Set defaults if not found
DAILY_CAP=${DAILY_CAP:-10000}
CPM=${CPM:-2.50}
BUDGET_MULTIPLIER=${BUDGET_MULTIPLIER:-1.2}

# Calculate budget amount: (daily_cap * cpm/1000 * multiplier)
BUDGET_AMOUNT=$(echo "scale=2; $DAILY_CAP * $CPM / 1000 * $BUDGET_MULTIPLIER" | bc)

echo "  Daily Impression Cap: $DAILY_CAP"
echo "  CPM: \$${CPM}"
echo "  Budget Multiplier: ${BUDGET_MULTIPLIER}x"
echo "  Calculated Budget: \$${BUDGET_AMOUNT} per line item"

# Replace placeholders in SQL and pipe to Postgres
if ! sed -e "s/{{DAILY_IMPRESSION_CAP}}/${DAILY_CAP}/g" \
         -e "s/{{CPM}}/${CPM}/g" \
         -e "s/{{BUDGET_AMOUNT}}/${BUDGET_AMOUNT}/g" "$SETUP_SQL" | \
    docker compose exec -T postgres psql -U postgres -d postgres -f /dev/stdin > /dev/null; then
    echo -e "${RED}❌ Failed to load test data${NC}"
    exit 1
fi

# Reload ad server configuration
echo -e "${BLUE}🔄 Reloading ad server configuration...${NC}"
if ! curl -s -X POST http://localhost:8787/reload > /dev/null; then
    echo -e "${YELLOW}⚠️  Warning: Could not reload ad server configuration${NC}"
fi

echo -e "${GREEN}✅ Test data loaded successfully${NC}"

# Verify test data with a sample request
echo -e "${BLUE}🧪 Verifying test setup...${NC}"
TEST_REQUEST='{"id":"test-req","imp":[{"id":"test-imp","tagid":"test-header","banner":{"w":728,"h":90}}],"user":{"id":"test123"},"device":{"ua":"test","ip":"192.0.2.1"},"ext":{"publisher_id":999}}'

if ! curl -s -X POST http://localhost:8787/ad \
    -H "Content-Type: application/json" \
    -H "X-API-Key: pacing-test-key" \
    -d "$TEST_REQUEST" | grep -q "seatbid"; then
    echo -e "${RED}❌ Test setup verification failed${NC}"
    echo "Ad server is not serving test ads properly"
    exit 1
fi

echo -e "${GREEN}✅ Test setup verified${NC}"

# Clear Redis for clean test
echo -e "${BLUE}🧹 Clearing Redis cache...${NC}"
docker compose exec -T redis redis-cli FLUSHALL > /dev/null

# Run traffic simulation
echo -e "${BLUE}🚀 Starting traffic simulation...${NC}"

# Read duration from config
DURATION=$(grep -A 10 "^traffic:" "$SCRIPT_DIR/config.yaml" | grep "duration:" | head -1 | sed 's/.*duration: *"\([^"]*\)".*/\1/')
if [ -z "$DURATION" ]; then
    DURATION="2h"  # fallback
fi

echo -e "${YELLOW}This will take approximately $DURATION to complete${NC}"
echo -e "${YELLOW}Monitor progress in another terminal with: docker compose logs -f openadserve${NC}"
echo ""

# Check if Python traffic generator is available
if command -v python3 > /dev/null && python3 -c "import yaml" 2> /dev/null; then
    echo -e "${GREEN}Using Python traffic generator${NC}"
    cd "$SCRIPT_DIR"
    python3 -c "
import sys
sys.path.append('../shared')
from traffic_generator import TrafficGenerator

generator = TrafficGenerator('config.yaml')
result = generator.run_from_config('pacing-comparison')
print(f'Traffic simulation completed with return code: {result.returncode}')
"
else
    echo -e "${YELLOW}Python dependencies not available, using Go traffic simulator directly${NC}"
    
    # Read additional config values for consistency
    TOTAL_REQUESTS=$(grep -A 10 "^traffic:" "$SCRIPT_DIR/config.yaml" | grep "total_requests:" | head -1 | sed 's/.*total_requests: *\([0-9]*\).*/\1/')
    USERS=$(grep -A 10 "^traffic:" "$SCRIPT_DIR/config.yaml" | grep "users:" | head -1 | sed 's/.*users: *\([0-9]*\).*/\1/')
    RATE=$(grep -A 10 "^traffic:" "$SCRIPT_DIR/config.yaml" | grep "rate_per_second:" | head -1 | sed 's/.*rate_per_second: *\([0-9]*\).*/\1/')
    CONCURRENCY=$(grep -A 10 "^traffic:" "$SCRIPT_DIR/config.yaml" | grep "concurrency:" | head -1 | sed 's/.*concurrency: *\([0-9]*\).*/\1/')
    CLICK_RATE=$(grep -A 10 "^traffic:" "$SCRIPT_DIR/config.yaml" | grep "click_rate:" | head -1 | sed 's/.*click_rate: *\([0-9.]*\).*/\1/')
    
    # Use defaults if not found
    TOTAL_REQUESTS=${TOTAL_REQUESTS:-500000}
    USERS=${USERS:-10000}
    RATE=${RATE:-250}
    CONCURRENCY=${CONCURRENCY:-100}
    CLICK_RATE=${CLICK_RATE:-0.05}
    
    # Fallback to direct Go execution
    go run "$PROJECT_ROOT/tools/traffic_simulator" \
        -server=http://localhost:8787 \
        -users="$USERS" \
        -requests="$TOTAL_REQUESTS" \
        -duration="$DURATION" \
        -rate="$RATE" \
        -concurrency="$CONCURRENCY" \
        -placements=test-header,test-sidebar,test-content \
        -api-key=pacing-test-key \
        -publisher-id=999 \
        -click-rate="$CLICK_RATE" \
        -stats \
        -label=pacing-comparison-test
fi

echo -e "${GREEN}✅ Traffic simulation completed${NC}"

# Analyze results
if [ "$SKIP_ANALYSIS" = true ]; then
    echo -e "${YELLOW}⏭️  Skipping analysis as requested${NC}"
else
    echo -e "${BLUE}📈 Analyzing results...${NC}"
fi

# Check if Python analysis is available
if [ "$SKIP_ANALYSIS" = false ]; then
    if command -v python3 > /dev/null && python3 -c "import matplotlib, pandas, yaml" 2> /dev/null; then
        echo -e "${GREEN}Generating detailed analysis and visualization${NC}"
        cd "$SCRIPT_DIR"
        python3 analyze.py
    else
        echo -e "${YELLOW}Python dependencies not available, generating basic report${NC}"

        # Basic CLI report
        echo ""
        echo "=== BASIC RESULTS ==="
        docker compose exec -T clickhouse clickhouse-client --query "
        SELECT
          campaign_id,
          CASE
            WHEN campaign_id = 999001 THEN 'ASAP Pacing'
            WHEN campaign_id = 999002 THEN 'Even Pacing'
            WHEN campaign_id = 999003 THEN 'PID Pacing'
          END as strategy,
          COUNT(*) as total_impressions
        FROM events
        WHERE campaign_id IN (999001, 999002, 999003)
        GROUP BY campaign_id
        ORDER BY total_impressions DESC
        " --format PrettyCompact
    fi
fi

# Optional cleanup after test (after analysis is complete)
if [ "$CLEANUP_AFTER" = true ]; then
    cleanup_test_data "after"
fi

echo ""
echo -e "${GREEN}"
echo "=============================================="
echo "  PACING STRATEGY TEST COMPLETED SUCCESSFULLY"
echo "=============================================="
echo -e "${NC}"

if [ "$SKIP_ANALYSIS" = false ]; then
    if [ -f "$SCRIPT_DIR/pacing_analysis.png" ] && [ -f "$SCRIPT_DIR/results.md" ]; then
        echo "📊 Results available at:"
        echo "  - Chart: $SCRIPT_DIR/pacing_analysis.png"
        echo "  - Report: $SCRIPT_DIR/results.md"
        echo ""
    else
        echo "📊 Basic results displayed above (Python dependencies not available for detailed analysis)"
        echo ""
    fi
fi

if [ "$CLEANUP_AFTER" = true ]; then
    echo "🧹 Test data and result files have been automatically cleaned up"
    echo "   (To keep result files, run without --cleanup-after or --cleanup-all)"
else
    echo "🧹 To clean up test data manually:"
    echo "  $0 --cleanup-after  # Use built-in cleanup"
    echo "  # Or manually:"
    echo "  docker compose exec postgres psql -U postgres -d postgres -c \"DELETE FROM events WHERE campaign_id IN (999001, 999002, 999003)\""
    echo "  docker compose exec postgres psql -U postgres -d postgres -c \"DELETE FROM creatives WHERE publisher_id = 999\""
    echo "  docker compose exec postgres psql -U postgres -d postgres -c \"DELETE FROM line_items WHERE publisher_id = 999\""
    echo "  docker compose exec postgres psql -U postgres -d postgres -c \"DELETE FROM campaigns WHERE publisher_id = 999\""
    echo "  docker compose exec postgres psql -U postgres -d postgres -c \"DELETE FROM placements WHERE publisher_id = 999\""
    echo "  docker compose exec postgres psql -U postgres -d postgres -c \"DELETE FROM publishers WHERE id = 999\""
fi
echo ""
echo "✅ Test framework ready for additional runs!"
echo "💡 Run '$0 --help' for more options"
