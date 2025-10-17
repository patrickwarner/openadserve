package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/observability"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	server          string
	users           int
	placementCSV    string
	totalReq        int
	conc            int
	duration        time.Duration
	rate            float64
	clickRate       float64
	stats           bool
	flush           bool
	redisAddr       string
	debug           bool
	label           string
	apiKey          string
	publisherID     int
	surgeInterval   time.Duration
	surgeDuration   time.Duration
	surgeMultiplier float64
	jitter          float64
	keyValues       string
)

var logger *zap.Logger

// HTTP client with proper resource limits
var httpClient *http.Client

// HTTP client for clicks that doesn't follow redirects
var clickClient *http.Client

var (
	placementIDs = []string{"header", "sidebar"}
	userAgents   = []string{
		// Mobile
		"Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Linux; Android 12; Pixel 6 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.5735.196 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 11; SAMSUNG SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/15.0 Chrome/94.0.4606.61 Mobile Safari/537.36",
		"Mozilla/5.0 (iPad; CPU OS 15_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.2 Mobile/15E148 Safari/604.1",

		// Desktop
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 13_3_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.1 Safari/605.1.15",
		"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:111.0) Gecko/20100101 Firefox/111.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 Edg/122.0.2365.66",
	}
	userIPs = []string{
		"192.0.2.1",
		"198.51.100.1",
		"203.0.113.1",
	}
)

const statsInterval = 5 * time.Second

var (
	countSent    uint64
	countSuccess uint64
	countNoBid   uint64
	countErrors  uint64
	countClicks  uint64
)

type ortbReq struct {
	ID     string            `json:"id"`
	Imp    []impObj          `json:"imp"`
	User   map[string]string `json:"user"`
	Device map[string]string `json:"device"`
	Ext    ortbExt           `json:"ext"`
}

type ortbExt struct {
	PublisherID int               `json:"publisher_id"`
	KV          map[string]string `json:"kv,omitempty"`
}
type impObj struct {
	ID    string `json:"id"`
	TagID string `json:"tagid"`
}

func main() {
	flag.StringVar(&server, "server", "http://localhost:8787", "ad server base URL")
	flag.IntVar(&users, "users", 100, "number of unique users")
	flag.StringVar(&placementCSV, "placements", "header,sidebar", "comma-separated placement IDs")
	flag.IntVar(&totalReq, "requests", 1000, "total requests to send")
	flag.IntVar(&conc, "concurrency", 20, "concurrent requests")
	flag.DurationVar(&duration, "duration", 0, "how long to run traffic (0 to disable)")
	flag.Float64Var(&rate, "rate", 0, "requests per second (0 for unlimited)")
	flag.Float64Var(&clickRate, "click-rate", 0.05, "probability of a click per impression")
	flag.BoolVar(&stats, "stats", false, "print aggregated stats periodically")
	flag.BoolVar(&flush, "flush", false, "flush redis before sending traffic")
	flag.StringVar(&redisAddr, "redis", "", "redis address (defaults to REDIS_ADDR)")
	flag.BoolVar(&debug, "debug", false, "enable verbose debug logs")
	flag.StringVar(&label, "label", "", "label to identify this run")
	flag.StringVar(&apiKey, "api-key", "demo123", "publisher API key")
	flag.IntVar(&publisherID, "publisher-id", 1, "publisher ID")
	flag.DurationVar(&surgeInterval, "surge-interval", 0, "interval between traffic surges (0 to disable)")
	flag.DurationVar(&surgeDuration, "surge-duration", 0, "duration of each surge window")
	flag.Float64Var(&surgeMultiplier, "surge-multiplier", 2.0, "requests multiplier during surge period")
	flag.Float64Var(&jitter, "jitter", 0.0, "random jitter factor for request spacing")
	flag.StringVar(&keyValues, "key-values", "", "comma-separated key=value pairs (e.g., category=sports,section=football)")
	flag.Parse()

	level := zapcore.InfoLevel
	if debug {
		level = zapcore.DebugLevel
	}
	var err error
	logger, err = observability.InitLoggerWithLevel(level, "traffic-simulator")
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	// Initialize HTTP client with proper resource limits
	httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			MaxConnsPerHost:       50, // Limit connections per host
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false, // Enable connection reuse
		},
	}

	// Initialize click client that doesn't follow redirects for testing
	clickClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if label == "" {
		label = time.Now().Format(time.RFC3339)
	}

	if flush {
		cfg := config.Load()
		addr := redisAddr
		if addr == "" {
			addr = cfg.RedisAddr
		}
		store, err := db.InitRedis(addr)
		if err != nil {
			logger.Fatal("redis connect", zap.Error(err))
		}

		// Selectively flush operational data only, preserve campaign data
		patterns := []string{
			"freqcap:*",     // frequency capping data
			"pacing:*",      // pacing counters
			"clicks:*",      // click counters
			"impressions:*", // impression counters
			"serves:*",      // serve counters
		}

		flushedCount := 0
		for _, pattern := range patterns {
			keys, err := store.Client.Keys(store.Ctx, pattern).Result()
			if err != nil {
				logger.Error("failed to get keys for pattern", zap.String("pattern", pattern), zap.Error(err))
				continue
			}
			if len(keys) > 0 {
				if err := store.Client.Del(store.Ctx, keys...).Err(); err != nil {
					logger.Error("failed to delete keys", zap.String("pattern", pattern), zap.Error(err))
					continue
				}
				flushedCount += len(keys)
			}
		}

		store.Close()
		logger.Info("redis operational data flushed",
			zap.String("addr", addr),
			zap.Int("keys_deleted", flushedCount),
			zap.String("note", "campaign data preserved"))
	}

	placementIDs = strings.Split(placementCSV, ",")
	for i := range placementIDs {
		placementIDs[i] = strings.TrimSpace(placementIDs[i])
	}

	// Parse key-values
	var parsedKV map[string]string
	if keyValues != "" {
		parsedKV = make(map[string]string)
		pairs := strings.Split(keyValues, ",")
		for _, pair := range pairs {
			parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(parts) == 2 {
				parsedKV[parts[0]] = parts[1]
			}
		}
		if len(parsedKV) > 0 {
			logger.Info("using key-values", zap.Any("kv", parsedKV))
		}
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var wg sync.WaitGroup
	sem := make(chan struct{}, conc)
	done := make(chan struct{})

	var baseInterval time.Duration
	if rate > 0 {
		baseInterval = time.Duration(float64(time.Second) / rate)
	} else if duration > 0 && totalReq > 0 {
		baseInterval = duration / time.Duration(totalReq)
	}

	start := time.Now()
	next := start

	if stats {
		go func() {
			ticker := time.NewTicker(statsInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					printStats()
				case <-done:
					printStats()
					return
				}
			}
		}()
	}
	for i := 0; ; i++ {
		if totalReq > 0 && i >= totalReq {
			break
		}
		if duration > 0 && time.Since(start) >= duration {
			break
		}
		if baseInterval > 0 {
			effective := baseInterval
			if surgeInterval > 0 && surgeDuration > 0 && surgeMultiplier > 0 {
				elapsed := time.Since(start)
				if elapsed%surgeInterval < surgeDuration {
					effective = time.Duration(float64(effective) / surgeMultiplier)
				}
			}
			if jitter > 0 {
				jf := 1 + (r.Float64()*2-1)*jitter
				if jf < 0.1 {
					jf = 0.1
				}
				effective = time.Duration(float64(effective) * jf)
			}
			now := time.Now()
			if now.Before(next) {
				time.Sleep(next.Sub(now))
			}
			next = next.Add(effective)
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			atomic.AddUint64(&countSent, 1)
			// build request
			// generate a random request ID similar to the SDK
			reqID := fmt.Sprintf("req_%s", strconv.FormatUint(r.Uint64(), 36))
			userID := fmt.Sprintf("user%d", r.Intn(users))
			placementID := placementIDs[r.Intn(len(placementIDs))]
			ua := userAgents[r.Intn(len(userAgents))]
			ip := userIPs[r.Intn(len(userIPs))]

			body := ortbReq{
				ID:     reqID,
				Imp:    []impObj{{ID: "1", TagID: placementID}},
				User:   map[string]string{"id": userID},
				Device: map[string]string{"ua": ua, "ip": ip},
				Ext:    ortbExt{PublisherID: publisherID, KV: parsedKV},
			}
			blob, err := json.Marshal(body)
			if err != nil {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("marshal error", zap.Error(err))
				return
			}
			// POST /ad
			req, err := http.NewRequest("POST", server+"/ad", bytes.NewReader(blob))
			if err != nil {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("request build error", zap.Error(err))
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", ip)
			if apiKey != "" {
				req.Header.Set("X-API-Key", apiKey)
			}
			// Add request timeout context
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			req = req.WithContext(ctx)

			resp, err := httpClient.Do(req)
			if err != nil {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("ad request error", zap.Error(err))
				return
			}
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("read body error", zap.Error(err))
				_ = resp.Body.Close()
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("unexpected status", zap.Int("status", resp.StatusCode), zap.String("body", strings.TrimSpace(string(bodyBytes))))
				return
			}
			var ortbRes struct {
				ID      string `json:"id"`
				SeatBid []struct {
					Bid []struct {
						ID       string `json:"id"`
						ImpID    string `json:"impid"`
						CrID     string `json:"crid"`
						CID      string `json:"cid"`
						ImpURL   string `json:"impurl"`
						ClickURL string `json:"clkurl"`
					} `json:"bid"`
				} `json:"seatbid"`
			}
			if err := json.Unmarshal(bodyBytes, &ortbRes); err != nil {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("decode error", zap.Error(err), zap.String("body", strings.TrimSpace(string(bodyBytes))))
				return
			}
			if err := resp.Body.Close(); err != nil {
				logger.Error("close body error", zap.Error(err))
			}

			if len(ortbRes.SeatBid) == 0 || len(ortbRes.SeatBid[0].Bid) == 0 {
				atomic.AddUint64(&countNoBid, 1)
				logger.Debug("no bid", zap.String("request_id", reqID))
				return
			}
			bid := ortbRes.SeatBid[0].Bid[0]

			// GET /impression tracking pixel
			impURL := strings.TrimRight(server, "/") + bid.ImpURL
			impCtx, impCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer impCancel()
			impReq, err := http.NewRequestWithContext(impCtx, "GET", impURL, nil)
			if err != nil {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("impression request build error", zap.Error(err))
				return
			}
			impResp, err := httpClient.Do(impReq)
			if err != nil {
				atomic.AddUint64(&countErrors, 1)
				logger.Error("impression get error", zap.Error(err))
				return
			}
			_ = impResp.Body.Close()
			if r.Float64() < clickRate && bid.ClickURL != "" {
				clkURL := strings.TrimRight(server, "/") + bid.ClickURL
				clkCtx, clkCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer clkCancel()
				clkReq, err := http.NewRequestWithContext(clkCtx, "GET", clkURL, nil)
				if err != nil {
					atomic.AddUint64(&countErrors, 1)
					logger.Error("click request build error", zap.Error(err))
					return
				}
				clkResp, err := clickClient.Do(clkReq)
				if err != nil {
					atomic.AddUint64(&countErrors, 1)
					logger.Error("click get error", zap.Error(err))
					return
				}
				_ = clkResp.Body.Close()
				atomic.AddUint64(&countClicks, 1)
			}
			atomic.AddUint64(&countSuccess, 1)
			logger.Debug("request", zap.String("req_id", reqID), zap.String("placement", placementID), zap.String("ip", ip), zap.String("ua", ua), zap.String("crid", bid.CrID))
		}(i)
	}
	wg.Wait()
	close(done)
	if !stats {
		printStats()
	}
}

func printStats() {
	sent := atomic.LoadUint64(&countSent)
	succ := atomic.LoadUint64(&countSuccess)
	nb := atomic.LoadUint64(&countNoBid)
	err := atomic.LoadUint64(&countErrors)
	clk := atomic.LoadUint64(&countClicks)
	var ctr float64
	if succ > 0 {
		ctr = float64(clk) / float64(succ)
	}
	logger.Info("stats", zap.String("run", label), zap.Uint64("sent", sent), zap.Uint64("success", succ), zap.Uint64("no_bid", nb), zap.Uint64("errors", err), zap.Uint64("clicks", clk), zap.Float64("ctr", ctr))
}
