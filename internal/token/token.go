package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("invalid token")
	ErrExpired = errors.New("token expired")
)

// Custom parameter validation limits to prevent token size issues
const (
	MaxCustomParamKeyLength   = 50
	MaxCustomParamValueLength = 100
	MaxCustomParamsCount      = 10
)

// payload structure for encoding/decoding
type payload struct {
	ReqID        string            `json:"r"`
	ImpID        string            `json:"i"`
	CrID         string            `json:"c"`
	CID          string            `json:"cid"`
	LIID         string            `json:"l"`   // Line Item ID
	UserID       string            `json:"u"`   // User ID
	PubID        string            `json:"p"`   // Publisher ID
	PlacementID  string            `json:"pl"`  // Placement ID
	BidPrice     float64           `json:"bp"`  // Auction price
	Currency     string            `json:"cur"` // Auction currency
	TS           int64             `json:"t"`
	CustomParams map[string]string `json:"cp,omitempty"` // Custom parameters for macro expansion
}

// validateCustomParams checks custom parameters against size limits to prevent token bloat
func validateCustomParams(params map[string]string) error {
	if len(params) > MaxCustomParamsCount {
		return fmt.Errorf("too many custom parameters: %d (max %d)", len(params), MaxCustomParamsCount)
	}

	for key, value := range params {
		if len(key) > MaxCustomParamKeyLength {
			return fmt.Errorf("custom param key too long: '%s' (%d chars, max %d)", key, len(key), MaxCustomParamKeyLength)
		}
		if len(value) > MaxCustomParamValueLength {
			return fmt.Errorf("custom param value too long for key '%s': '%s' (%d chars, max %d)", key, value, len(value), MaxCustomParamValueLength)
		}
		if key == "" {
			return fmt.Errorf("custom param key cannot be empty")
		}
	}
	return nil
}

// Generate creates a signed token for the given identifiers.
func Generate(requestID, impID, crID, cid, liid, userID, pubID string, secret []byte) (string, error) {
	return GenerateWithAuctionData(requestID, impID, crID, cid, liid, userID, pubID, "", 0.0, "USD", nil, secret)
}

// GenerateWithCustomParams creates a signed token for the given identifiers including custom parameters.
func GenerateWithCustomParams(requestID, impID, crID, cid, liid, userID, pubID string, customParams map[string]string, secret []byte) (string, error) {
	return GenerateWithAuctionData(requestID, impID, crID, cid, liid, userID, pubID, "", 0.0, "USD", customParams, secret)
}

// GenerateWithAuctionData creates a signed token for the given identifiers including auction data and custom parameters.
func GenerateWithAuctionData(requestID, impID, crID, cid, liid, userID, pubID, placementID string, bidPrice float64, currency string, customParams map[string]string, secret []byte) (string, error) {
	// Validate custom parameters to prevent token size issues
	if customParams != nil {
		if err := validateCustomParams(customParams); err != nil {
			return "", fmt.Errorf("custom parameter validation failed: %w", err)
		}
	}

	pl := payload{
		ReqID:        requestID,
		ImpID:        impID,
		CrID:         crID,
		CID:          cid,
		LIID:         liid,
		UserID:       userID,
		PubID:        pubID,
		PlacementID:  placementID,
		BidPrice:     bidPrice,
		Currency:     currency,
		TS:           time.Now().Unix(),
		CustomParams: customParams,
	}
	data, err := json.Marshal(pl)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	sig := mac.Sum(nil)

	enc := base64.RawURLEncoding
	token := enc.EncodeToString(data) + "." + enc.EncodeToString(sig)
	return token, nil
}

// Verify checks the token integrity and expiry and returns the payload values.
func Verify(token string, secret []byte, ttl time.Duration) (out struct {
	RequestID    string
	ImpID        string
	CrID         string
	CID          string
	LIID         string
	UserID       string
	PubID        string
	PlacementID  string
	BidPrice     float64
	Currency     string
	CustomParams map[string]string
}, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return out, ErrInvalid
	}
	enc := base64.RawURLEncoding
	data, err := enc.DecodeString(parts[0])
	if err != nil {
		return out, ErrInvalid
	}
	sig, err := enc.DecodeString(parts[1])
	if err != nil {
		return out, ErrInvalid
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return out, ErrInvalid
	}

	var pl payload
	if err := json.Unmarshal(data, &pl); err != nil {
		return out, ErrInvalid
	}
	if ttl > 0 && time.Since(time.Unix(pl.TS, 0)) > ttl {
		return out, ErrExpired
	}
	out.RequestID = pl.ReqID
	out.ImpID = pl.ImpID
	out.CrID = pl.CrID
	out.CID = pl.CID
	out.LIID = pl.LIID
	out.UserID = pl.UserID
	out.PubID = pl.PubID
	out.PlacementID = pl.PlacementID
	out.BidPrice = pl.BidPrice
	out.Currency = pl.Currency
	out.CustomParams = pl.CustomParams
	return out, nil
}
