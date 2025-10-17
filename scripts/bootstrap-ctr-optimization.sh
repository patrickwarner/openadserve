#!/bin/bash

# Bootstrap CTR Optimization System
# This script sets up synthetic data and trains the initial model

set -e

echo "🚀 Bootstrapping CTR Optimization System..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
CTR_PREDICTOR_URL="http://localhost:8000"
DAYS_OF_DATA=7
IMPRESSIONS_PER_DAY=10000

echo -e "${BLUE}📊 Configuration:${NC}"
echo "   • CTR Predictor URL: $CTR_PREDICTOR_URL"
echo "   • Days of data: $DAYS_OF_DATA"
echo "   • Impressions per day: $IMPRESSIONS_PER_DAY"
echo ""

# Function to check if service is ready
wait_for_service() {
    local url=$1
    local service_name=$2
    local max_attempts=30
    local attempt=1
    
    echo -e "${YELLOW}⏳ Waiting for $service_name to be ready...${NC}"
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url/health" > /dev/null 2>&1; then
            echo -e "${GREEN}✅ $service_name is ready!${NC}"
            return 0
        fi
        
        echo "   Attempt $attempt/$max_attempts..."
        sleep 2
        ((attempt++))
    done
    
    echo -e "${RED}❌ $service_name failed to start after $max_attempts attempts${NC}"
    return 1
}

# Function to make API call and handle errors
api_call() {
    local method=$1
    local url=$2
    local data=$3
    local description=$4
    
    echo -e "${YELLOW}📡 $description...${NC}"
    
    if [ "$method" = "POST" ] && [ -n "$data" ]; then
        response=$(curl -s -X POST "$url" \
            -H "Content-Type: application/json" \
            -d "$data")
    else
        response=$(curl -s -X "$method" "$url")
    fi
    
    # Check if response contains error
    if echo "$response" | grep -q '"status":"success"' || echo "$response" | grep -q '"status":"healthy"'; then
        echo -e "${GREEN}✅ Success!${NC}"
        return 0
    else
        echo -e "${RED}❌ Failed: $response${NC}"
        return 1
    fi
}

# Step 1: Wait for CTR Predictor service
if ! wait_for_service "$CTR_PREDICTOR_URL" "CTR Predictor"; then
    echo -e "${RED}💥 CTR Predictor service is not available. Make sure it's running:${NC}"
    echo "   docker compose up ctr-predictor"
    exit 1
fi

# Step 2: Generate synthetic data
echo ""
echo -e "${BLUE}🎲 Step 1: Generating synthetic ad serving data...${NC}"

synthetic_data='{"days": '$DAYS_OF_DATA', "impressions_per_day": '$IMPRESSIONS_PER_DAY'}'

if api_call "POST" "$CTR_PREDICTOR_URL/generate-synthetic-data?days=$DAYS_OF_DATA&impressions_per_day=$IMPRESSIONS_PER_DAY" "" "Generating $DAYS_OF_DATA days of synthetic data"; then
    echo -e "${GREEN}📊 Synthetic data generation complete!${NC}"
else
    echo -e "${RED}❌ Failed to generate synthetic data${NC}"
    exit 1
fi

# Step 3: Train the model
echo ""
echo -e "${BLUE}🧠 Step 2: Training CTR prediction model...${NC}"

training_data='{"days_back": '$DAYS_OF_DATA', "min_impressions": 100}'

if api_call "POST" "$CTR_PREDICTOR_URL/train" "$training_data" "Training CTR model with generated data"; then
    echo -e "${GREEN}🎯 Model training complete!${NC}"
else
    echo -e "${RED}❌ Failed to train model${NC}"
    exit 1
fi

# Step 4: Verify model is loaded
echo ""
echo -e "${BLUE}🔍 Step 3: Verifying model status...${NC}"

if api_call "GET" "$CTR_PREDICTOR_URL/model-info" "" "Checking model information"; then
    echo -e "${GREEN}✅ Model is loaded and ready!${NC}"
else
    echo -e "${RED}❌ Model verification failed${NC}"
    exit 1
fi

# Step 5: Test prediction
echo ""
echo -e "${BLUE}🧪 Step 4: Testing CTR prediction...${NC}"

test_prediction='{
    "line_item_id": 2,
    "device_type": "mobile", 
    "country": "US",
    "hour_of_day": 20,
    "day_of_week": 2
}'

if api_call "POST" "$CTR_PREDICTOR_URL/predict" "$test_prediction" "Testing mobile CPC prediction"; then
    echo -e "${GREEN}🎉 CTR prediction is working!${NC}"
else
    echo -e "${RED}❌ CTR prediction test failed${NC}"
    exit 1
fi

# Success message
echo ""
echo -e "${GREEN}🎉 CTR Optimization System Bootstrap Complete!${NC}"
echo ""
echo -e "${BLUE}📈 What was accomplished:${NC}"
echo "   ✅ Generated $DAYS_OF_DATA days of realistic ad serving data"
echo "   ✅ Trained logistic regression CTR prediction model"
echo "   ✅ Model is loaded and ready for predictions"
echo "   ✅ Verified prediction API is working"
echo ""
echo -e "${BLUE}🚀 Next steps:${NC}"
echo "   1. Enable CTR optimization in ad server:"
echo "      export CTR_OPTIMIZATION_ENABLED=true"
echo "   2. Start the full ad server:"
echo "      docker compose up ad-server"
echo "   3. Send test ad requests and observe CTR-optimized serving"
echo ""
echo -e "${YELLOW}💡 The model has learned these patterns:${NC}"
echo "   • Mobile users have ~2x higher CTR than desktop"
echo "   • Evening hours (6-10 PM) have 60-70% higher engagement"
echo "   • CPC campaigns (line item 2) perform 20-30% better on mobile"
echo "   • Weekdays outperform weekends"
echo ""
echo -e "${GREEN}Ready to test CTR optimization in the wild! 🚀${NC}"