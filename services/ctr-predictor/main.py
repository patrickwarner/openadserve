"""
CTR Prediction Service for Ad Server CPC Optimization

This service provides click-through rate predictions for CPC line items
using logistic regression trained on historical click data.
"""

import os
import logging
import pickle
import random
from contextlib import asynccontextmanager
from datetime import datetime, timedelta
from typing import Dict, Optional, Tuple
from functools import lru_cache

import pandas as pd
import numpy as np
from sklearn.linear_model import LogisticRegression
from sklearn.preprocessing import StandardScaler, LabelEncoder
from sklearn.model_selection import train_test_split
from sklearn.metrics import classification_report, roc_auc_score
import clickhouse_driver
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, ConfigDict
import uvicorn

# Configure logging
def get_log_level():
    env = os.getenv("ENV", "production").lower()
    log_level = os.getenv("LOG_LEVEL", "").upper()
    
    if log_level:
        return getattr(logging, log_level, logging.INFO)
    
    if env in ["development", "dev"]:
        return logging.DEBUG
    elif env in ["staging", "test"]:
        return logging.INFO
    else:  # production
        return logging.INFO

def get_sampling_rate():
    env = os.getenv("ENV", "production").lower()
    if env in ["development", "dev"]:
        return 1.0  # No sampling in development
    elif env in ["staging", "test"]:
        return 0.5  # 50% sampling in staging
    else:  # production
        return 0.1  # 10% sampling in production

def should_sample(rate=None):
    if rate is None:
        rate = get_sampling_rate()
    if rate >= 1.0:
        return True
    if rate <= 0.0:
        return False
    return random.random() < rate

logging.basicConfig(level=get_log_level())
logger = logging.getLogger(__name__)

# Configuration
CLICKHOUSE_HOST = os.getenv("CLICKHOUSE_HOST", "localhost")
CLICKHOUSE_PORT = int(os.getenv("CLICKHOUSE_PORT", "9000"))
CLICKHOUSE_DB = os.getenv("CLICKHOUSE_DB", "default")
MODEL_PATH = "/app/models/ctr_model.pkl"
SCALER_PATH = "/app/models/scaler.pkl"
ENCODERS_PATH = "/app/models/encoders.pkl"

# Global model variables
model = None
scaler = None
encoders = None

class PredictionRequest(BaseModel):
    line_item_id: int
    device_type: str
    country: str
    hour_of_day: int
    day_of_week: int
    publisher_id: Optional[int] = None

class PredictionResponse(BaseModel):
    line_item_id: int
    ctr_score: float
    confidence: float
    boost_multiplier: float

class TrainingRequest(BaseModel):
    days_back: int = 7
    min_impressions: int = 100

class TrainingResponse(BaseModel):
    model_config = ConfigDict(protected_namespaces=())
    
    status: str
    samples_trained: int
    model_accuracy: float
    auc_score: float

def get_clickhouse_client():
    """Get ClickHouse client connection."""
    return clickhouse_driver.Client(
        host=CLICKHOUSE_HOST,
        port=CLICKHOUSE_PORT,
        database=CLICKHOUSE_DB
    )

def load_models():
    """Load trained models from disk."""
    global model, scaler, encoders
    
    try:
        if os.path.exists(MODEL_PATH):
            with open(MODEL_PATH, 'rb') as f:
                model = pickle.load(f)
            logger.info("Loaded CTR prediction model")
        
        if os.path.exists(SCALER_PATH):
            with open(SCALER_PATH, 'rb') as f:
                scaler = pickle.load(f)
            logger.info("Loaded feature scaler")
        
        if os.path.exists(ENCODERS_PATH):
            with open(ENCODERS_PATH, 'rb') as f:
                encoders = pickle.load(f)
            logger.info("Loaded label encoders")
    
    except Exception as e:
        logger.error(f"Error loading models: {e}")
        model = scaler = encoders = None

def save_models():
    """Save trained models to disk."""
    os.makedirs(os.path.dirname(MODEL_PATH), exist_ok=True)
    
    with open(MODEL_PATH, 'wb') as f:
        pickle.dump(model, f)
    
    with open(SCALER_PATH, 'wb') as f:
        pickle.dump(scaler, f)
    
    with open(ENCODERS_PATH, 'wb') as f:
        pickle.dump(encoders, f)
    
    logger.info("Models saved to disk")

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Manage application lifespan events."""
    # Startup: Load models
    logger.info("Loading models on startup...")
    load_models()
    yield
    # Shutdown: Could add cleanup here if needed
    logger.info("Application shutdown")

# Initialize FastAPI app with lifespan
app = FastAPI(title="CTR Prediction Service", version="1.0.0", lifespan=lifespan)

def extract_features(df: pd.DataFrame) -> pd.DataFrame:
    """Extract and engineer features from raw event data."""
    # Extract time-based features
    df['hour_of_day'] = df['timestamp'].dt.hour
    df['day_of_week'] = df['timestamp'].dt.dayofweek
    df['is_weekend'] = df['day_of_week'].isin([5, 6]).astype(int)
    
    # Business hours feature
    df['is_business_hours'] = ((df['hour_of_day'] >= 9) & (df['hour_of_day'] <= 17)).astype(int)
    
    # Evening hours feature (high engagement)
    df['is_evening'] = ((df['hour_of_day'] >= 18) & (df['hour_of_day'] <= 22)).astype(int)
    
    return df

def prepare_training_data(days_back: int = 7, min_impressions: int = 100) -> pd.DataFrame:
    """Fetch and prepare training data from ClickHouse."""
    client = get_clickhouse_client()
    
    # Calculate date range
    end_date = datetime.now()
    start_date = end_date - timedelta(days=days_back)
    
    # Query to get impression and click events with full context
    query = """
    SELECT 
        line_item_id,
        timestamp,
        event_type,
        device_type,
        country,
        publisher_id,
        toHour(timestamp) as hour_of_day,
        toDayOfWeek(timestamp) as day_of_week
    FROM events 
    WHERE timestamp >= %(start_date)s 
        AND timestamp <= %(end_date)s
        AND line_item_id IS NOT NULL
        AND device_type IS NOT NULL
        AND country IS NOT NULL
        AND event_type IN ('impression', 'click')
    ORDER BY timestamp
    """
    
    logger.info(f"Querying events from {start_date} to {end_date}")
    
    try:
        result = client.execute(query, {
            'start_date': start_date,
            'end_date': end_date
        })
        
        df = pd.DataFrame(result, columns=[
            'line_item_id', 'timestamp', 'event_type', 'device_type', 'country', 'publisher_id', 'hour_of_day', 'day_of_week'
        ])
        
        if should_sample():
            logger.info(f"Fetched {len(df)} events")
        
        if df.empty:
            logger.warning("No events found for training")
            return pd.DataFrame()
        
        # Convert timestamp to datetime
        df['timestamp'] = pd.to_datetime(df['timestamp'])
        
        # Group by line_item_id and all context features to create training samples
        # Each sample represents a line item's performance in a specific context
        training_data = []
        
        for line_item_id in df['line_item_id'].unique():
            li_df = df[df['line_item_id'] == line_item_id]
            
            # Group by all contextual features to create context-specific samples
            for (hour, day, device, country, publisher), group in li_df.groupby(['hour_of_day', 'day_of_week', 'device_type', 'country', 'publisher_id']):
                impressions = len(group[group['event_type'] == 'impression'])
                clicks = len(group[group['event_type'] == 'click'])
                
                # Only include samples with sufficient impressions
                if impressions >= min_impressions:
                    # Create positive samples (clicked impressions)
                    for _ in range(clicks):
                        training_data.append({
                            'line_item_id': line_item_id,
                            'device_type': device,
                            'country': country,
                            'publisher_id': publisher,
                            'hour_of_day': hour,
                            'day_of_week': day,
                            'clicked': 1
                        })
                    
                    # Create negative samples (non-clicked impressions)
                    # Sample to balance the dataset
                    non_clicks = impressions - clicks
                    sample_size = min(non_clicks, clicks * 3)  # Up to 3:1 ratio
                    
                    for _ in range(sample_size):
                        training_data.append({
                            'line_item_id': line_item_id,
                            'device_type': device,
                            'country': country,
                            'publisher_id': publisher,
                            'hour_of_day': hour,
                            'day_of_week': day,
                            'clicked': 0
                        })
        
        training_df = pd.DataFrame(training_data)
        if should_sample():
            logger.info(f"Created {len(training_df)} training samples")
        
        return training_df
        
    except Exception as e:
        logger.error(f"Error preparing training data: {e}")
        raise

def train_model(training_df: pd.DataFrame) -> Tuple[float, float]:
    """Train logistic regression model on prepared data."""
    global model, scaler, encoders
    
    if training_df.empty:
        raise ValueError("No training data available")
    
    logger.info("Starting model training")
    
    # Prepare features including contextual data
    features = ['line_item_id', 'device_type', 'country', 'publisher_id', 'hour_of_day', 'day_of_week']
    X = training_df[features].copy()
    y = training_df['clicked']
    
    # Encode categorical features
    encoders = {}
    for col in ['line_item_id', 'device_type', 'country', 'publisher_id']:
        le = LabelEncoder()
        X[col] = le.fit_transform(X[col].astype(str))  # Convert to string to handle nulls
        encoders[col] = le
    
    # Engineer additional features
    X['is_weekend'] = X['day_of_week'].isin([5, 6]).astype(int)
    X['is_business_hours'] = ((X['hour_of_day'] >= 9) & (X['hour_of_day'] <= 17)).astype(int)
    X['is_evening'] = ((X['hour_of_day'] >= 18) & (X['hour_of_day'] <= 22)).astype(int)
    
    # Scale features
    scaler = StandardScaler()
    X_scaled = scaler.fit_transform(X)
    
    # Split data
    X_train, X_test, y_train, y_test = train_test_split(
        X_scaled, y, test_size=0.2, random_state=42, stratify=y
    )
    
    # Train model
    model = LogisticRegression(random_state=42, max_iter=1000)
    model.fit(X_train, y_train)
    
    # Evaluate model
    y_pred = model.predict(X_test)
    y_pred_proba = model.predict_proba(X_test)[:, 1]
    
    accuracy = model.score(X_test, y_test)
    auc = roc_auc_score(y_test, y_pred_proba)
    
    logger.info(f"Model trained - Accuracy: {accuracy:.3f}, AUC: {auc:.3f}")
    logger.info("\nClassification Report:")
    logger.info(classification_report(y_test, y_pred))
    
    return accuracy, auc

# Cache frequent predictions to improve performance
@lru_cache(maxsize=1000)
def predict_ctr_cached(line_item_id: int, device_type: str, country: str, publisher_id: int, hour_of_day: int, day_of_week: int) -> Tuple[float, float]:
    """Cached CTR prediction for identical requests."""
    return predict_ctr_internal(line_item_id, device_type, country, publisher_id, hour_of_day, day_of_week)

def predict_ctr(line_item_id: int, device_type: str, country: str, publisher_id: Optional[int], hour_of_day: int, day_of_week: int) -> Tuple[float, float]:
    """Predict CTR with caching for identical requests.""" 
    # Use cached version with normalized publisher_id
    return predict_ctr_cached(line_item_id, device_type, country, publisher_id or 0, hour_of_day, day_of_week)

def predict_ctr_internal(line_item_id: int, device_type: str, country: str, publisher_id: int, hour_of_day: int, day_of_week: int) -> Tuple[float, float]:
    """Predict CTR for given line item and context."""
    global model, scaler, encoders
    
    if model is None or scaler is None or encoders is None:
        raise ValueError("Model not trained or loaded")
    
    try:
        # Prepare features as numpy array for better performance
        features = np.zeros(9)  # 6 original features + 3 engineered features
        
        # Encode categorical features directly
        features[0] = line_item_id
        if encoders.get('line_item_id') and str(line_item_id) in encoders['line_item_id'].classes_:
            features[0] = encoders['line_item_id'].transform([str(line_item_id)])[0]
        
        if encoders.get('device_type') and device_type in encoders['device_type'].classes_:
            features[1] = encoders['device_type'].transform([device_type])[0]
        
        if encoders.get('country') and country in encoders['country'].classes_:
            features[2] = encoders['country'].transform([country])[0]
        
        pub_id = publisher_id or 0
        if encoders.get('publisher_id') and str(pub_id) in encoders['publisher_id'].classes_:
            features[3] = encoders['publisher_id'].transform([str(pub_id)])[0]
        
        features[4] = hour_of_day
        features[5] = day_of_week
        
        # Engineer additional features directly
        features[6] = 1 if day_of_week in [5, 6] else 0  # is_weekend
        features[7] = 1 if 9 <= hour_of_day <= 17 else 0  # is_business_hours  
        features[8] = 1 if 18 <= hour_of_day <= 22 else 0  # is_evening
        
        # Scale features
        features_scaled = scaler.transform(features.reshape(1, -1))
        
        # Make prediction
        probabilities = model.predict_proba(features_scaled)[0]
        probability = probabilities[1]
        confidence = max(probabilities)
        
        return probability, confidence
        
    except Exception as e:
        if should_sample():
            logger.error(f"Error in prediction: {e}")
        return 0.01, 0.5  # Default low CTR with medium confidence


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {
        "status": "healthy",
        "model_loaded": model is not None,
        "timestamp": datetime.now().isoformat()
    }

@app.post("/predict", response_model=PredictionResponse)
async def predict(request: PredictionRequest):
    """Predict CTR for a line item in given context."""
    if model is None:
        raise HTTPException(status_code=503, detail="Model not trained")
    
    try:
        ctr_score, confidence = predict_ctr(
            request.line_item_id,
            request.device_type,
            request.country,
            request.publisher_id,
            request.hour_of_day,
            request.day_of_week
        )
        
        # Convert CTR to boost multiplier
        # Higher CTR gets higher boost, capped at 2.0x
        baseline_ctr = 0.01  # 1% baseline
        boost_multiplier = min(ctr_score / baseline_ctr, 2.0)
        boost_multiplier = max(boost_multiplier, 0.5)  # Minimum 0.5x
        
        return PredictionResponse(
            line_item_id=request.line_item_id,
            ctr_score=ctr_score,
            confidence=confidence,
            boost_multiplier=boost_multiplier
        )
        
    except Exception as e:
        if should_sample():
            logger.error(f"Prediction error: {e}")
        raise HTTPException(status_code=500, detail="Prediction failed")

@app.post("/train", response_model=TrainingResponse)
async def train_model_endpoint(request: TrainingRequest):
    """Train or retrain the CTR prediction model."""
    try:
        if should_sample():
            logger.info(f"Starting training with {request.days_back} days of data")
        
        # Prepare training data
        training_df = prepare_training_data(
            days_back=request.days_back,
            min_impressions=request.min_impressions
        )
        
        if training_df.empty:
            raise HTTPException(status_code=400, detail="No training data available")
        
        # Train model
        accuracy, auc = train_model(training_df)
        
        # Save models
        save_models()
        
        return TrainingResponse(
            status="success",
            samples_trained=len(training_df),
            model_accuracy=accuracy,
            auc_score=auc
        )
        
    except Exception as e:
        logger.error(f"Training error: {e}")
        raise HTTPException(status_code=500, detail=f"Training failed: {str(e)}")

@app.get("/model-info")
async def model_info():
    """Get information about the current model."""
    if model is None:
        return {"status": "no_model", "message": "No model loaded"}
    
    return {
        "status": "loaded",
        "model_type": type(model).__name__,
        "features_count": len(model.coef_[0]) if hasattr(model, 'coef_') else None,
        "classes": model.classes_.tolist() if hasattr(model, 'classes_') else None
    }

@app.post("/generate-synthetic-data")
async def generate_synthetic_data_endpoint(days: int = 7, impressions_per_day: int = 10000):
    """Generate synthetic ad serving data for model training."""
    try:
        from synthetic_data import SyntheticDataGenerator
        
        logger.info(f"Generating {days} days of synthetic data...")
        
        generator = SyntheticDataGenerator(
            clickhouse_host=CLICKHOUSE_HOST,
            clickhouse_port=CLICKHOUSE_PORT, 
            clickhouse_db=CLICKHOUSE_DB
        )
        
        generator.generate_synthetic_data(days=days, impressions_per_day=impressions_per_day)
        generator.analyze_generated_data()
        
        return {
            "status": "success",
            "message": f"Generated {days} days of synthetic data",
            "impressions_per_day": impressions_per_day,
            "total_estimated_events": days * impressions_per_day * 1.3  # ~30% CTR means 1.3x events
        }
        
    except Exception as e:
        logger.error(f"Error generating synthetic data: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to generate synthetic data: {str(e)}")

if __name__ == "__main__":
    # Use multiple workers for better concurrency
    import multiprocessing
    workers = min(multiprocessing.cpu_count(), 4)  # Cap at 4 workers
    uvicorn.run(
        "main:app", 
        host="0.0.0.0", 
        port=8000,
        workers=workers,
        worker_class="uvicorn.workers.UvicornWorker"
    )