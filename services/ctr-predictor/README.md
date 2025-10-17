# CTR Prediction Service

A Python-based service that provides click-through rate predictions for CPC line items using logistic regression.

## Features

- **CTR Prediction**: Predicts click probability based on line item, time, and context
- **Model Training**: Trains models using historical ClickHouse data
- **Feature Engineering**: Extracts time-based and contextual features
- **REST API**: FastAPI-based service with OpenAPI documentation
- **Docker Support**: Containerized deployment

## API Endpoints

### Health Check
```
GET /health
```

### Predict CTR
```
POST /predict
{
  "line_item_id": 123,
  "device_type": "mobile",
  "country": "US",
  "hour_of_day": 14,
  "day_of_week": 2
}
```

Response:
```json
{
  "line_item_id": 123,
  "ctr_score": 0.025,
  "confidence": 0.8,
  "boost_multiplier": 2.5
}
```

### Train Model
```
POST /train
{
  "days_back": 7,
  "min_impressions": 100
}
```

### Model Information
```
GET /model-info
```

## Environment Variables

- `CLICKHOUSE_HOST`: ClickHouse server host (default: localhost)
- `CLICKHOUSE_PORT`: ClickHouse server port (default: 9000)
- `CLICKHOUSE_DB`: ClickHouse database name (default: default)

## Development

```bash
# Install dependencies
pip install -r requirements.txt

# Run locally
python main.py

# Build Docker image
docker build -t ctr-predictor .

# Run container
docker run -p 8000:8000 ctr-predictor
```

## Model Architecture

The service uses a logistic regression model with the following features:
- Line item ID (encoded)
- Hour of day (0-23)
- Day of week (0-6)
- Is weekend (boolean)
- Is business hours (9-17)
- Is evening hours (18-22)

The model is trained on historical impression/click data and provides boost multipliers for eCPM calculations.