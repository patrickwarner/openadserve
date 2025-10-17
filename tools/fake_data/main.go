package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
)

var (
	pubCount     = flag.Int("publishers", 1, "number of publishers")
	campPerPub   = flag.Int("campaigns", 20, "campaigns per publisher")
	liPerCamp    = flag.Int("lineitems", 3, "line items per campaign")
	creativesPer = flag.Int("creatives", 2, "creatives per line item")
	placements   = flag.Int("placements", 2, "placements per publisher")
	seed         = flag.Int64("seed", time.Now().UnixNano(), "rng seed")
	skipReload   = flag.Bool("skip-reload", false, "skip automatic reload after data insertion")
)

func main() {
	flag.Parse()

	logger, err := observability.InitLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	cfg := config.Load()
	pg, err := db.InitPostgres(cfg.PostgresDSN, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.DBConnMaxLifetime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pg.Close()

	r := rand.New(rand.NewSource(*seed))

	// Check if demo publisher already exists
	var demoExists int
	if err := pg.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM publishers WHERE domain = 'demo.example.com'`).Scan(&demoExists); err != nil {
		logger.Fatal("check demo publisher", zap.Error(err))
	}

	if demoExists == 0 {
		demo := models.Publisher{Name: "Demo Publisher", Domain: "demo.example.com", APIKey: "demo123"}
		if err := insertPublisher(pg, &demo); err != nil {
			logger.Fatal("insert demo publisher", zap.Error(err))
		}

		pls := demoPlacements(demo.ID)
		for i := range pls {
			if err := insertPlacement(pg, pls[i]); err != nil {
				logger.Fatal("insert placement", zap.Error(err))
			}
		}

		for c := 0; c < *campPerPub; c++ {
			camp := models.Campaign{PublisherID: demo.ID, Name: fakeCampaignName(r)}
			if err := insertCampaign(pg, &camp); err != nil {
				logger.Fatal("insert campaign", zap.Error(err))
			}

			for l := 0; l < *liPerCamp; l++ {
				li := demoLineItem(r, camp.ID, demo.ID)
				if err := insertLineItem(pg, &li); err != nil {
					logger.Fatal("insert line item", zap.Error(err))
				}

				for x := 0; x < *creativesPer; x++ {
					// Only use display placements for random campaigns, not native ones
					displayPls := []models.Placement{}
					for _, p := range pls {
						if len(p.Formats) > 0 && p.Formats[0] == "html" { // Only include display ads
							displayPls = append(displayPls, p)
						}
					}
					if len(displayPls) > 0 {
						pl := displayPls[r.Intn(len(displayPls))]
						cr := randomCreative(r, li.ID, camp.ID, demo.ID, pl)
						if err := insertCreative(pg, &cr); err != nil {
							logger.Fatal("insert creative", zap.Error(err))
						}
					}
				}
			}
		}

		// Add specific native ad campaigns for social media demo
		if err := createNativeAdCampaigns(pg, demo.ID, pls); err != nil {
			logger.Fatal("create native ad campaigns", zap.Error(err))
		}
	}

	for i := 0; i < *pubCount; i++ {
		pub := models.Publisher{
			Name:   fakeName(r),
			Domain: fakeDomain(r),
			APIKey: randomString(r, 8),
		}
		if err := insertPublisher(pg, &pub); err != nil {
			logger.Fatal("insert publisher", zap.Error(err))
		}

		pls := make([]models.Placement, *placements)
		for j := 0; j < *placements; j++ {
			p := randomPlacement(r, pub.ID, j+1)
			pls[j] = p
			if err := insertPlacement(pg, p); err != nil {
				logger.Fatal("insert placement", zap.Error(err))
			}
		}

		for c := 0; c < *campPerPub; c++ {
			camp := models.Campaign{PublisherID: pub.ID, Name: fakeCampaignName(r)}
			if err := insertCampaign(pg, &camp); err != nil {
				logger.Fatal("insert campaign", zap.Error(err))
			}

			for l := 0; l < *liPerCamp; l++ {
				li := randomLineItem(r, camp.ID, pub.ID)
				if err := insertLineItem(pg, &li); err != nil {
					logger.Fatal("insert line item", zap.Error(err))
				}

				for x := 0; x < *creativesPer; x++ {
					pl := pls[r.Intn(len(pls))]
					cr := randomCreative(r, li.ID, camp.ID, pub.ID, pl)
					if err := insertCreative(pg, &cr); err != nil {
						logger.Fatal("insert creative", zap.Error(err))
					}
				}
			}
		}
	}

	fmt.Println("fake data inserted")

	if !*skipReload {
		if err := callReloadEndpoint(&cfg); err != nil {
			logger.Error("reload endpoint failed", zap.Error(err))
			fmt.Fprintf(os.Stderr, "Warning: failed to reload server data: %v\n", err)
		} else {
			fmt.Println("server data reloaded")
		}
	}
}

func insertPublisher(pg *db.Postgres, p *models.Publisher) error {
	err := pg.DB.QueryRowContext(context.Background(), `INSERT INTO publishers (name, domain, api_key) VALUES ($1,$2,$3) RETURNING id`, p.Name, p.Domain, p.APIKey).Scan(&p.ID)
	return err
}

func insertCampaign(pg *db.Postgres, c *models.Campaign) error {
	err := pg.DB.QueryRowContext(context.Background(), `INSERT INTO campaigns (publisher_id, name) VALUES ($1,$2) RETURNING id`, c.PublisherID, c.Name).Scan(&c.ID)
	return err
}

func insertPlacement(pg *db.Postgres, p models.Placement) error {
	_, err := pg.DB.ExecContext(context.Background(), `INSERT INTO placements (id, publisher_id, width, height, formats) VALUES ($1,$2,$3,$4,$5)`, p.ID, p.PublisherID, p.Width, p.Height, pq.Array(p.Formats))
	return err
}

func insertLineItem(pg *db.Postgres, li *models.LineItem) error {
	var kv interface{}
	if len(li.KeyValues) > 0 {
		b, _ := json.Marshal(li.KeyValues)
		kv = string(b)
	}
	err := pg.DB.QueryRowContext(context.Background(), `INSERT INTO line_items (
        campaign_id, publisher_id, name, start_date, end_date, daily_impression_cap, daily_click_cap,
        pace_type, priority, frequency_cap, frequency_window, country, device_type, os, browser,
        active, key_values, cpm, cpc, ecpm, budget_type, budget_amount, spend, li_type, endpoint, click_url)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26) RETURNING id`,
		li.CampaignID, li.PublisherID, li.Name, nullTime(li.StartDate), nullTime(li.EndDate), li.DailyImpressionCap, li.DailyClickCap,
		nullString(li.PaceType), nullString(li.Priority), li.FrequencyCap, int(li.FrequencyWindow.Seconds()),
		nullString(li.Country), nullString(li.DeviceType), nullString(li.OS), nullString(li.Browser),
		li.Active, kv, li.CPM, li.CPC, li.ECPM, nullString(li.BudgetType), li.BudgetAmount, li.Spend,
		nullString(li.Type), nullString(li.Endpoint), nullString(li.ClickURL)).Scan(&li.ID)
	return err
}

func insertCreative(pg *db.Postgres, c *models.Creative) error {
	var native interface{}
	if len(c.Native) > 0 {
		native = string(c.Native)
	} else {
		native = nil
	}
	err := pg.DB.QueryRowContext(context.Background(), `INSERT INTO creatives (
        placement_id, line_item_id, campaign_id, publisher_id, html, native, width, height, format, click_url)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		c.PlacementID, c.LineItemID, c.CampaignID, c.PublisherID, c.HTML, native, c.Width, c.Height, c.Format, nullString(c.ClickURL)).Scan(&c.ID)
	return err
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// random helpers

var nameAdjectives = []string{"Acme", "Prime", "Dynamic", "Next", "Fast", "Bright", "Super"}
var nameNouns = []string{"Media", "Ads", "Network", "Marketing", "Solutions", "Labs"}

func fakeName(r *rand.Rand) string {
	return fmt.Sprintf("%s %s", nameAdjectives[r.Intn(len(nameAdjectives))], nameNouns[r.Intn(len(nameNouns))])
}

var domainWords = []string{"alpha", "beta", "gamma", "delta", "omega", "ad", "market"}
var domainTLDs = []string{"com", "net", "io", "dev"}

func fakeDomain(r *rand.Rand) string {
	return fmt.Sprintf("%s%d.%s", domainWords[r.Intn(len(domainWords))], r.Intn(1000), domainTLDs[r.Intn(len(domainTLDs))])
}

func randomString(r *rand.Rand, n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

func fakeCampaignName(r *rand.Rand) string {
	seasons := []string{"Spring", "Summer", "Fall", "Winter", "Holiday"}
	products := []string{"Sale", "Launch", "Promo", "Special"}
	return fmt.Sprintf("%s %s %d", seasons[r.Intn(len(seasons))], products[r.Intn(len(products))], r.Intn(100))
}

// fakeLineItemName generates a somewhat descriptive name for a line item so
// dropdowns in the admin UI show meaningful labels.
func fakeLineItemName(r *rand.Rand) string {
	channels := []string{"Homepage", "Sidebar", "Content", "In-App", "Video"}
	mediums := []string{"Banner", "Native", "Interstitial", "Feed", "Popup"}
	return fmt.Sprintf("%s %s %d", channels[r.Intn(len(channels))], mediums[r.Intn(len(mediums))], r.Intn(1000))
}

// generateRealisticClickURL creates realistic advertiser landing page URLs with appropriate macro usage
func generateRealisticClickURL(r *rand.Rand) string {
	// Different types of advertisers and their typical URL structures
	advertisers := []struct {
		domain         string
		landingPage    string
		trackingParams []string
	}{
		{
			domain:      "example.com",
			landingPage: "/sale",
			trackingParams: []string{
				"utm_source={PUBLISHER_ID}",
				"utm_medium=display",
				"utm_campaign={CAMPAIGN_ID}",
				"utm_content={CREATIVE_ID}",
				"click_id={UUID}",
				"timestamp={TIMESTAMP}",
			},
		},
		{
			domain:      "example.com",
			landingPage: "/products",
			trackingParams: []string{
				"src={PUBLISHER_ID}",
				"campaign={CAMPAIGN_ID}",
				"creative={CREATIVE_ID}",
				"auction={AUCTION_ID}",
			},
		},
		{
			domain:      "example.com",
			landingPage: "/offers",
			trackingParams: []string{
				"publisher={PUBLISHER_ID}",
				"camp={CAMPAIGN_ID}",
				"cr={CREATIVE_ID}",
				"uid={UUID}",
				"t={TIMESTAMP_MS}",
			},
		},
		{
			domain:      "example.com",
			landingPage: "/wellness",
			trackingParams: []string{
				"utm_source={PUBLISHER_ID}",
				"utm_medium=native",
				"utm_campaign={CAMPAIGN_ID}",
				"gclid={AUCTION_ID}",
				"session_id={UUID}",
			},
		},
		{
			domain:      "example.com",
			landingPage: "/collection",
			trackingParams: []string{
				"ref={PUBLISHER_ID}",
				"cid={CAMPAIGN_ID}",
				"aid={CREATIVE_ID}",
				"ts={TIMESTAMP}",
				"rnd={RANDOM}",
			},
		},
		{
			domain:      "example.com",
			landingPage: "/download",
			trackingParams: []string{
				"affiliate={PUBLISHER_ID}",
				"campaign={CAMPAIGN_ID}",
				"banner={CREATIVE_ID}",
				"clickid={UUID}",
			},
		},
		{
			domain:      "example.com",
			landingPage: "/destinations",
			trackingParams: []string{
				"partner={PUBLISHER_ID}",
				"promo={CAMPAIGN_ID}",
				"ad={CREATIVE_ID}",
				"uuid={UUID}",
				"when={ISO_TIMESTAMP}",
			},
		},
	}

	// Select random advertiser
	advertiser := advertisers[r.Intn(len(advertisers))]

	// Build URL with tracking parameters
	baseURL := fmt.Sprintf("https://%s%s", advertiser.domain, advertiser.landingPage)

	// Add some random subset of tracking parameters (2-5 params)
	numParams := r.Intn(4) + 2 // 2-5 parameters
	if numParams > len(advertiser.trackingParams) {
		numParams = len(advertiser.trackingParams)
	}

	// Shuffle and select parameters
	params := make([]string, len(advertiser.trackingParams))
	copy(params, advertiser.trackingParams)

	// Fisher-Yates shuffle
	for i := len(params) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		params[i], params[j] = params[j], params[i]
	}

	// Build query string
	var queryParams []string
	for i := 0; i < numParams; i++ {
		queryParams = append(queryParams, params[i])
	}

	if len(queryParams) > 0 {
		return baseURL + "?" + strings.Join(queryParams, "&")
	}

	return baseURL
}

func randomPlacement(r *rand.Rand, pubID, idx int) models.Placement {
	sizes := []struct {
		Name string
		W, H int
	}{
		{"leaderboard", 728, 90},
		{"rectangle", 300, 250},
		{"skyscraper", 160, 600},
		{"mobile_banner", 320, 50},
		{"square", 250, 250},
	}
	s := sizes[r.Intn(len(sizes))]
	fmts := []string{"html"}
	if r.Intn(3) == 0 {
		fmts = append(fmts, "native")
	}
	id := fmt.Sprintf("%s_%d_%d", s.Name, pubID, idx)
	return models.Placement{ID: id, PublisherID: pubID, Width: s.W, Height: s.H, Formats: fmts}
}

func demoPlacements(pubID int) []models.Placement {
	return []models.Placement{
		{ID: "header", PublisherID: pubID, Width: 728, Height: 90, Formats: []string{"html"}},
		{ID: "sidebar", PublisherID: pubID, Width: 160, Height: 600, Formats: []string{"html"}},
		{ID: "content_rect", PublisherID: pubID, Width: 300, Height: 250, Formats: []string{"html"}},
		{ID: "native_feed_simple", PublisherID: pubID, Width: 0, Height: 0, Formats: []string{"native"}},
		{ID: "social_native_post", PublisherID: pubID, Width: 0, Height: 0, Formats: []string{"native"}},
		{ID: "social_skin_takeover", PublisherID: pubID, Width: 0, Height: 0, Formats: []string{"native"}},
	}
}

var deviceTypes = []string{"mobile", "desktop", "tablet"}
var oses = []string{"iOS", "Android", "Windows", "macOS"}
var browsers = []string{"Safari", "Chrome", "Firefox", "Edge"}
var countries = []string{"US", "CA", "GB", "DE", "FR"}
var paceTypes = []string{models.PacingASAP, models.PacingEven}
var priorities = []string{models.PriorityHigh, models.PriorityMedium, models.PriorityLow}
var budgetTypes = []string{models.BudgetTypeCPM, models.BudgetTypeCPC}
var lineItemTypes = []string{models.LineItemTypeDirect, models.LineItemTypeProgrammatic}

func demoLineItem(r *rand.Rand, campID, pubID int) models.LineItem {
	// ensure line items are immediately active by starting in the past
	start := time.Now().Add(-time.Duration(r.Intn(72)) * time.Hour)
	end := start.Add(time.Duration(r.Intn(21)+7) * 24 * time.Hour)
	li := models.LineItem{
		CampaignID:         campID,
		PublisherID:        pubID,
		Name:               fakeLineItemName(r),
		StartDate:          start,
		EndDate:            end,
		DailyImpressionCap: r.Intn(5000),
		PaceType:           paceTypes[r.Intn(len(paceTypes))],
		Priority:           priorities[r.Intn(len(priorities))],
		FrequencyCap:       3,
		FrequencyWindow:    60 * time.Second,
		Active:             true,
		BudgetType:         budgetTypes[r.Intn(len(budgetTypes))],
		BudgetAmount:       float64(r.Intn(8000) + 2000),
		Type:               lineItemTypes[r.Intn(len(lineItemTypes))],
		ClickURL:           generateRealisticClickURL(r),
		// No targeting constraints for demo publisher to ensure ads serve
	}
	switch li.BudgetType {
	case models.BudgetTypeCPM:
		li.CPM = float64(r.Intn(500)+50) / 100
		li.ECPM = li.CPM
	case models.BudgetTypeCPC:
		li.CPC = float64(r.Intn(200)+25) / 100
		ctr := 0.02 + r.Float64()*0.05
		li.ECPM = li.CPC * ctr * 1000
	}
	if li.Type == models.LineItemTypeProgrammatic {
		li.Endpoint = fmt.Sprintf("https://buyer%d.example.com/bid", r.Intn(10))
	}
	return li
}

func randomLineItem(r *rand.Rand, campID, pubID int) models.LineItem {
	// ensure line items are immediately active by starting in the past
	start := time.Now().Add(-time.Duration(r.Intn(72)) * time.Hour)
	end := start.Add(time.Duration(r.Intn(21)+7) * 24 * time.Hour)
	li := models.LineItem{
		CampaignID:         campID,
		PublisherID:        pubID,
		Name:               fakeLineItemName(r),
		StartDate:          start,
		EndDate:            end,
		DailyImpressionCap: r.Intn(5000),
		PaceType:           paceTypes[r.Intn(len(paceTypes))],
		Priority:           priorities[r.Intn(len(priorities))],
		FrequencyCap:       3,
		FrequencyWindow:    60 * time.Second,
		Active:             true,
		BudgetType:         budgetTypes[r.Intn(len(budgetTypes))],
		BudgetAmount:       float64(r.Intn(8000) + 2000),
		Type:               lineItemTypes[r.Intn(len(lineItemTypes))],
		ClickURL:           generateRealisticClickURL(r),
	}
	if r.Intn(2) == 0 {
		li.Country = countries[r.Intn(len(countries))]
	}
	if r.Intn(2) == 0 {
		li.DeviceType = deviceTypes[r.Intn(len(deviceTypes))]
	}
	if r.Intn(2) == 0 {
		li.OS = oses[r.Intn(len(oses))]
	}
	if r.Intn(2) == 0 {
		li.Browser = browsers[r.Intn(len(browsers))]
	}
	switch li.BudgetType {
	case models.BudgetTypeCPM:
		li.CPM = float64(r.Intn(500)+50) / 100
		li.ECPM = li.CPM
	case models.BudgetTypeCPC:
		li.CPC = float64(r.Intn(200)+25) / 100
		ctr := 0.02 + r.Float64()*0.05
		li.ECPM = li.CPC * ctr * 1000
	}
	if li.Type == models.LineItemTypeProgrammatic {
		li.Endpoint = fmt.Sprintf("https://buyer%d.example.com/bid", r.Intn(10))
	}
	return li
}

func randomCreative(r *rand.Rand, liID, campID, pubID int, p models.Placement) models.Creative {
	randID := r.Intn(10000)
	txt := fmt.Sprintf("Ad %d", randID)
	clickURL := generateRealisticClickURL(r)

	if len(p.Formats) > 0 && p.Formats[0] == "native" {
		data := map[string]string{
			"title": txt,
			"image": fmt.Sprintf("https://example.com/image%d.png", randID),
			"click": clickURL,
		}
		b, _ := json.Marshal(data)
		return models.Creative{
			PlacementID: p.ID,
			LineItemID:  liID,
			CampaignID:  campID,
			PublisherID: pubID,
			Native:      b,
			Width:       p.Width,
			Height:      p.Height,
			Format:      "native",
			ClickURL:    clickURL,
		}
	}
	html := fmt.Sprintf("<div style='width:%dpx;height:%dpx;background:#e8f4ff;border:1px solid #888;display:flex;align-items:center;justify-content:center;font-family:sans-serif;cursor:pointer;'>%s</div>", p.Width, p.Height, txt)
	return models.Creative{
		PlacementID: p.ID,
		LineItemID:  liID,
		CampaignID:  campID,
		PublisherID: pubID,
		HTML:        html,
		Width:       p.Width,
		Height:      p.Height,
		Format:      p.Formats[0],
		ClickURL:    clickURL,
	}
}

func callReloadEndpoint(cfg *config.Config) error {
	reloadURL := fmt.Sprintf("http://localhost:%s/reload", cfg.Port)
	req, err := http.NewRequest("POST", reloadURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return nil
}

func createNativeAdCampaigns(pg *db.Postgres, publisherID int, placements []models.Placement) error {
	// Unified FitLife Pro Campaign - Holiday Multi-Format Package
	fitLifeCamp := models.Campaign{PublisherID: publisherID, Name: "FitLife Pro - Holiday Wellness Multi-Format Package"}
	if err := insertCampaign(pg, &fitLifeCamp); err != nil {
		return err
	}

	// Line Item 1: Social Feed Native Ad
	socialFeedLI := models.LineItem{
		CampaignID:         fitLifeCamp.ID,
		PublisherID:        publisherID,
		Name:               "FitLife Pro Social Feed Native - Holiday Sale",
		StartDate:          time.Now().Add(-24 * time.Hour),
		EndDate:            time.Now().Add(30 * 24 * time.Hour),
		DailyImpressionCap: 0,
		PaceType:           models.PacingEven,
		Priority:           models.PriorityHigh,
		FrequencyCap:       0,
		FrequencyWindow:    0,
		Active:             true,
		BudgetType:         models.BudgetTypeCPM,
		BudgetAmount:       8000.00,
		CPM:                5.50,
		ECPM:               5.50,
		Type:               models.LineItemTypeDirect,
		ClickURL:           "https://example.com/holiday-sale?utm_source={PUBLISHER_ID}&utm_medium=native&utm_campaign={CAMPAIGN_ID}&utm_content={CREATIVE_ID}&auction_id={AUCTION_ID}",
	}
	if err := insertLineItem(pg, &socialFeedLI); err != nil {
		return err
	}

	// Creative 1: Social Media Feed Format
	socialFeedData := map[string]string{
		"title":       "FitLife Pro Holiday Wellness Sale",
		"brand":       "FitLife Pro",
		"description": "Ready to crush your 2024 fitness goals? 🏋️‍♀️ Get personalized workout plans, nutrition guidance, and expert coaching. Join our community of 100K+ success stories! Holiday special: 60% off premium memberships.",
		"image":       "https://images.unsplash.com/photo-1571019613454-1cb2f99b2d8b?w=600&h=200&fit=crop&crop=center",
		"click":       "https://example.com/holiday-sale?utm_source={PUBLISHER_ID}&utm_medium=native_feed&utm_campaign={CAMPAIGN_ID}&utm_content={CREATIVE_ID}&user_id={UUID}&timestamp={TIMESTAMP}",
		"cta_text":    "Get 60% Off - Start Today",
	}
	socialFeedJSON, _ := json.Marshal(socialFeedData)
	socialFeedCreative := models.Creative{
		PlacementID: "social_native_post",
		LineItemID:  socialFeedLI.ID,
		CampaignID:  fitLifeCamp.ID,
		PublisherID: publisherID,
		Native:      socialFeedJSON,
		Width:       0,
		Height:      0,
		Format:      "native",
		ClickURL:    "https://example.com/holiday-sale?utm_source={PUBLISHER_ID}&utm_medium=native_feed&utm_campaign={CAMPAIGN_ID}&utm_content={CREATIVE_ID}&user_id={UUID}&timestamp={TIMESTAMP}",
	}
	if err := insertCreative(pg, &socialFeedCreative); err != nil {
		return err
	}

	// Line Item 2: Background Skin Takeover - Premium Placement
	takeoverLI := models.LineItem{
		CampaignID:         fitLifeCamp.ID,
		PublisherID:        publisherID,
		Name:               "FitLife Pro Background Takeover - Holiday Sale",
		StartDate:          time.Now().Add(-24 * time.Hour),
		EndDate:            time.Now().Add(30 * 24 * time.Hour),
		DailyImpressionCap: 0,
		PaceType:           models.PacingASAP,
		Priority:           models.PriorityHigh,
		FrequencyCap:       0,
		FrequencyWindow:    0,
		Active:             true,
		BudgetType:         models.BudgetTypeCPM,
		BudgetAmount:       20000.00,
		CPM:                15.00,
		ECPM:               15.00,
		Type:               models.LineItemTypeDirect,
		ClickURL:           "https://example.com/premium-landing?utm_source={PUBLISHER_ID}&utm_medium=takeover&utm_campaign={CAMPAIGN_ID}&placement={PLACEMENT_ID}&session={UUID}",
	}
	if err := insertLineItem(pg, &takeoverLI); err != nil {
		return err
	}

	// Creative 3: Background Skin Takeover
	takeoverData := map[string]string{
		"title":       "FitLife Pro Holiday Wellness Sale",
		"brand":       "FitLife Pro",
		"description": "New Year, New You! Start your fitness transformation with personalized training and nutrition. Limited time: 60% off all premium plans. Join 100,000+ success stories!",
		"image":       "https://images.unsplash.com/photo-1571019613454-1cb2f99b2d8b?w=800&h=600&fit=crop&crop=center",
		"background":  "https://images.unsplash.com/photo-1571019613454-1cb2f99b2d8b?w=1920&h=1080&fit=crop&crop=center",
		"cta_text":    "Start Your Journey - 60% Off",
		"click":       "https://example.com/premium-landing?utm_source={PUBLISHER_ID}&utm_medium=takeover&utm_campaign={CAMPAIGN_ID}&placement={PLACEMENT_ID}&session={UUID}",
		"offer":       "60%",
	}
	takeoverJSON, _ := json.Marshal(takeoverData)
	takeoverCreative := models.Creative{
		PlacementID: "social_skin_takeover",
		LineItemID:  takeoverLI.ID,
		CampaignID:  fitLifeCamp.ID,
		PublisherID: publisherID,
		Native:      takeoverJSON,
		Width:       0,
		Height:      0,
		Format:      "native",
		ClickURL:    "https://example.com/premium-landing?utm_source={PUBLISHER_ID}&utm_medium=takeover&utm_campaign={CAMPAIGN_ID}&placement={PLACEMENT_ID}&session={UUID}",
	}
	if err := insertCreative(pg, &takeoverCreative); err != nil {
		return err
	}

	return nil
}
