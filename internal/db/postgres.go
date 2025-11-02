package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// Postgres wraps a postgres DB connection.
type Postgres struct {
	DB *sql.DB
}

// schemaSQL sets up the necessary tables if they don't exist.
const schemaSQL = `CREATE TABLE IF NOT EXISTS publishers (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    domain TEXT NOT NULL,
    api_key TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS campaigns (
    id SERIAL PRIMARY KEY,
    publisher_id INT REFERENCES publishers(id),
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS placements (
    id TEXT PRIMARY KEY,
    publisher_id INT REFERENCES publishers(id),
    width INT,
    height INT,
    formats TEXT[]
);

CREATE TABLE IF NOT EXISTS line_items (
    id SERIAL PRIMARY KEY,
    campaign_id INT NOT NULL,
    publisher_id INT REFERENCES publishers(id),
    name TEXT NOT NULL,
    start_date TIMESTAMP NULL,
    end_date TIMESTAMP NULL,
    daily_impression_cap INT NOT NULL,
    daily_click_cap INT NOT NULL,
    pace_type TEXT,
    priority TEXT,
    frequency_cap INT,
    frequency_window INT,
    country TEXT,
    device_type TEXT,
    os TEXT,
    browser TEXT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    key_values JSONB,
    cpm DOUBLE PRECISION,
    cpc DOUBLE PRECISION,
    ecpm DOUBLE PRECISION,
    budget_type TEXT,
    budget_amount DOUBLE PRECISION,
    spend DOUBLE PRECISION,
    li_type TEXT,
    endpoint TEXT,
    click_url TEXT
);

CREATE TABLE IF NOT EXISTS creatives (
    id SERIAL PRIMARY KEY,
    placement_id TEXT REFERENCES placements(id),
    line_item_id INT REFERENCES line_items(id),
    campaign_id INT REFERENCES campaigns(id),
    publisher_id INT REFERENCES publishers(id),
    html TEXT,
    native JSONB,
    banner JSONB,
    width INT,
    height INT,
    format TEXT,
    click_url TEXT
);

CREATE TABLE IF NOT EXISTS report_reasons (
    code VARCHAR(50) PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,
    description TEXT,
    severity VARCHAR(20) DEFAULT 'medium',
    auto_block_threshold INTEGER DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS ad_reports (
    id SERIAL PRIMARY KEY,
    creative_id INTEGER REFERENCES creatives(id),
    line_item_id INTEGER REFERENCES line_items(id),
    campaign_id INTEGER REFERENCES campaigns(id),
    publisher_id INTEGER REFERENCES publishers(id),
    user_id VARCHAR(255),
    placement_id TEXT REFERENCES placements(id),
    report_reason VARCHAR(50) NOT NULL,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status VARCHAR(20) DEFAULT 'pending'
);

-- Performance indexes for ad serving
CREATE INDEX IF NOT EXISTS idx_line_items_active_dates ON line_items (active, start_date, end_date) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_creatives_placement_id ON creatives (placement_id);
CREATE INDEX IF NOT EXISTS idx_creatives_line_item_id ON creatives (line_item_id);
CREATE INDEX IF NOT EXISTS idx_line_items_campaign_id ON line_items (campaign_id);
CREATE INDEX IF NOT EXISTS idx_line_items_publisher_id ON line_items (publisher_id);
CREATE INDEX IF NOT EXISTS idx_publishers_api_key ON publishers (api_key);
CREATE INDEX IF NOT EXISTS idx_campaigns_publisher_id ON campaigns (publisher_id);
CREATE INDEX IF NOT EXISTS idx_placements_publisher_id ON placements (publisher_id);
`

// InitPostgres connects to Postgres with connection pooling configuration.
func InitPostgres(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetime, connMaxIdleTime time.Duration) (*Postgres, error) {
	// Register the otelsql wrapper for postgres
	driverName, err := otelsql.Register("postgres",
		otelsql.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.connection_string", dsn),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("register otelsql: %w", err)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	// Configure connection pooling for production use
	db.SetMaxOpenConns(maxOpenConns)       // Maximum number of open connections
	db.SetMaxIdleConns(maxIdleConns)       // Maximum number of idle connections
	db.SetConnMaxLifetime(connMaxLifetime) // Maximum lifetime of a connection
	db.SetConnMaxIdleTime(connMaxIdleTime) // Maximum idle time before closing connection

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	p := &Postgres{DB: db}
	if err := p.ensureSchema(); err != nil {
		return nil, err
	}
	if err := p.ensureReportReasons(); err != nil {
		return nil, err
	}
	zap.L().Info("Connected to Postgres with connection pooling",
		zap.Int("max_open_conns", maxOpenConns),
		zap.Int("max_idle_conns", maxIdleConns),
		zap.Duration("conn_max_lifetime", connMaxLifetime))
	return p, nil
}

// Close terminates the Postgres connection.
func (p *Postgres) Close() {
	if p != nil && p.DB != nil {
		if err := p.DB.Close(); err != nil {
			zap.L().Error("postgres close", zap.Error(err))
		}
	}
}

// ensureSchema creates the required tables if they do not exist.
func (p *Postgres) ensureSchema() error {
	ctx := context.Background()
	if _, err := p.DB.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

// ensureReportReasons inserts default report reasons if none exist.
func (p *Postgres) ensureReportReasons() error {
	ctx := context.Background()
	var count int
	if err := p.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM report_reasons`).Scan(&count); err != nil {
		return fmt.Errorf("count report_reasons: %w", err)
	}
	if count > 0 {
		return nil
	}
	reasons := []models.ReportReason{
		{Code: "offensive", DisplayName: "Offensive or inappropriate", Description: "Contains offensive or inappropriate content", Severity: "high"},
		{Code: "malware", DisplayName: "Malware or security risk", Description: "Links to malware or attempts phishing", Severity: "critical"},
		{Code: "misleading", DisplayName: "Misleading or scam", Description: "Deceptive or fraudulent ad", Severity: "high"},
		{Code: "irrelevant", DisplayName: "Irrelevant", Description: "Not relevant to the page content", Severity: "low"},
		{Code: "other", DisplayName: "Other", Description: "Other issue", Severity: "medium"},
	}
	for _, rr := range reasons {
		if _, err := p.DB.ExecContext(ctx, `INSERT INTO report_reasons (code, display_name, description, severity, auto_block_threshold) VALUES ($1,$2,$3,$4,$5)`, rr.Code, rr.DisplayName, rr.Description, rr.Severity, rr.AutoBlockThreshold); err != nil {
			return fmt.Errorf("insert report reason %s: %w", rr.Code, err)
		}
	}
	return nil
}

// LoadLineItems retrieves active line items from the database.
func (p *Postgres) LoadLineItems() ([]models.LineItem, error) {
	rows, err := p.DB.QueryContext(context.Background(), `SELECT id, campaign_id, publisher_id, name, start_date, end_date, daily_impression_cap, daily_click_cap, pace_type, priority, frequency_cap, frequency_window, country, device_type, os, browser, active, key_values, cpm, cpc, ecpm, budget_type, budget_amount, spend, li_type, endpoint, click_url FROM line_items WHERE active AND (start_date IS NULL OR start_date <= NOW()) AND (end_date IS NULL OR end_date >= NOW())`)
	if err != nil {
		return nil, fmt.Errorf("query line items: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var items []models.LineItem
	for rows.Next() {
		var li models.LineItem
		var start, end sql.NullTime
		var freq sql.NullInt64
		var kv sql.NullString
		var pace, priority, country, deviceType, osVal, browser sql.NullString
		var active bool
		var budgetType, liType, endpoint, clickURL sql.NullString
		if err := rows.Scan(&li.ID, &li.CampaignID, &li.PublisherID, &li.Name, &start, &end, &li.DailyImpressionCap, &li.DailyClickCap, &pace, &priority, &li.FrequencyCap, &freq, &country, &deviceType, &osVal, &browser, &active, &kv, &li.CPM, &li.CPC, &li.ECPM, &budgetType, &li.BudgetAmount, &li.Spend, &liType, &endpoint, &clickURL); err != nil {
			return nil, fmt.Errorf("scan line item: %w", err)
		}
		if pace.Valid {
			li.PaceType = pace.String
		}
		if priority.Valid {
			li.Priority = priority.String
		}
		if country.Valid {
			li.Country = country.String
		}
		if deviceType.Valid {
			li.DeviceType = deviceType.String
		}
		if osVal.Valid {
			li.OS = osVal.String
		}
		if browser.Valid {
			li.Browser = browser.String
		}
		li.Active = active
		if li.ECPM == 0 {
			li.ECPM = li.CPM
		}
		if budgetType.Valid {
			li.BudgetType = budgetType.String
		}
		if liType.Valid {
			li.Type = liType.String
		}
		if endpoint.Valid {
			li.Endpoint = endpoint.String
		}
		if clickURL.Valid {
			li.ClickURL = clickURL.String
		}
		if start.Valid {
			li.StartDate = start.Time
		}
		if end.Valid {
			li.EndDate = end.Time
		}
		if freq.Valid {
			li.FrequencyWindow = time.Duration(freq.Int64) * time.Second
		}
		if kv.Valid {
			if err := json.Unmarshal([]byte(kv.String), &li.KeyValues); err != nil {
				return nil, fmt.Errorf("parse key_values: %w", err)
			}
		}
		items = append(items, li)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return items, nil
}

// LoadCampaigns retrieves campaigns from the database and returns them.
func (p *Postgres) LoadCampaigns() ([]models.Campaign, error) {
	rows, err := p.DB.QueryContext(context.Background(), `SELECT id, publisher_id, name FROM campaigns`)
	if err != nil {
		return nil, fmt.Errorf("query campaigns: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var cs []models.Campaign
	for rows.Next() {
		var c models.Campaign
		if err := rows.Scan(&c.ID, &c.PublisherID, &c.Name); err != nil {
			return nil, fmt.Errorf("scan campaign: %w", err)
		}
		cs = append(cs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return cs, nil
}

// LoadPlacements fetches placement definitions from the database.
func (p *Postgres) LoadPlacements() ([]models.Placement, error) {
	rows, err := p.DB.QueryContext(context.Background(), `SELECT id, publisher_id, width, height, formats FROM placements`)
	if err != nil {
		return nil, fmt.Errorf("query placements: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var pls []models.Placement
	for rows.Next() {
		var pl models.Placement
		var formats []string
		if err := rows.Scan(&pl.ID, &pl.PublisherID, &pl.Width, &pl.Height, pq.Array(&formats)); err != nil {
			return nil, fmt.Errorf("scan placement: %w", err)
		}
		pl.Formats = formats
		pls = append(pls, pl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return pls, nil
}

// LoadCreatives fetches creatives from the database.
func (p *Postgres) LoadCreatives() ([]models.Creative, error) {
	rows, err := p.DB.QueryContext(context.Background(), `SELECT id, placement_id, line_item_id, campaign_id, publisher_id, html, native, banner, width, height, format, click_url FROM creatives`)
	if err != nil {
		return nil, fmt.Errorf("query creatives: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var cs []models.Creative
	for rows.Next() {
		var c models.Creative
		var native, banner, clickURL sql.NullString
		if err := rows.Scan(&c.ID, &c.PlacementID, &c.LineItemID, &c.CampaignID, &c.PublisherID, &c.HTML, &native, &banner, &c.Width, &c.Height, &c.Format, &clickURL); err != nil {
			return nil, fmt.Errorf("scan creative: %w", err)
		}
		if native.Valid {
			c.Native = json.RawMessage(native.String)
		}
		if banner.Valid {
			c.Banner = json.RawMessage(banner.String)
		}
		if clickURL.Valid {
			c.ClickURL = clickURL.String
		}
		cs = append(cs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return cs, nil
}

// LoadPublishers fetches publishers from the database.
func (p *Postgres) LoadPublishers() ([]models.Publisher, error) {
	rows, err := p.DB.QueryContext(context.Background(), `SELECT id, name, domain, api_key FROM publishers`)
	if err != nil {
		return nil, fmt.Errorf("query publishers: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var pubs []models.Publisher
	for rows.Next() {
		var pub models.Publisher
		if err := rows.Scan(&pub.ID, &pub.Name, &pub.Domain, &pub.APIKey); err != nil {
			return nil, fmt.Errorf("scan publisher: %w", err)
		}
		pubs = append(pubs, pub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return pubs, nil
}

// InsertPublisher inserts a new publisher record and returns the generated ID.
func (p *Postgres) InsertPublisher(pub *models.Publisher) error {
	err := p.DB.QueryRowContext(context.Background(), `INSERT INTO publishers (name, domain, api_key) VALUES ($1,$2,$3) RETURNING id`, pub.Name, pub.Domain, pub.APIKey).Scan(&pub.ID)
	if err != nil {
		return fmt.Errorf("insert publisher: %w", err)
	}
	return nil
}

// InsertAdReport stores a new ad report from a user.
func (p *Postgres) InsertAdReport(r models.AdReport) error {
	_, err := p.DB.ExecContext(context.Background(), `INSERT INTO ad_reports (
            creative_id, line_item_id, campaign_id, publisher_id, user_id,
            placement_id, report_reason, ip_address, user_agent, status)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		r.CreativeID, r.LineItemID, r.CampaignID, r.PublisherID, r.UserID,
		r.PlacementID, r.ReportReason, r.IPAddress, r.UserAgent, r.Status)
	if err != nil {
		return fmt.Errorf("insert ad report: %w", err)
	}
	return nil
}

// UpdatePublisher updates an existing publisher.
func (p *Postgres) UpdatePublisher(pub models.Publisher) error {
	_, err := p.DB.ExecContext(context.Background(), `UPDATE publishers SET name=$1, domain=$2, api_key=$3 WHERE id=$4`, pub.Name, pub.Domain, pub.APIKey, pub.ID)
	if err != nil {
		return fmt.Errorf("update publisher: %w", err)
	}
	return nil
}

// DeletePublisher removes a publisher by ID.
func (p *Postgres) DeletePublisher(id int) error {
	_, err := p.DB.ExecContext(context.Background(), `DELETE FROM publishers WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete publisher: %w", err)
	}
	return nil
}

// InsertCampaign inserts a new campaign and returns the generated ID.
func (p *Postgres) InsertCampaign(c *models.Campaign) error {
	err := p.DB.QueryRowContext(context.Background(), `INSERT INTO campaigns (publisher_id, name) VALUES ($1,$2) RETURNING id`, c.PublisherID, c.Name).Scan(&c.ID)
	if err != nil {
		return fmt.Errorf("insert campaign: %w", err)
	}
	return nil
}

// UpdateCampaign updates an existing campaign.
func (p *Postgres) UpdateCampaign(c models.Campaign) error {
	_, err := p.DB.ExecContext(context.Background(), `UPDATE campaigns SET publisher_id=$1, name=$2 WHERE id=$3`, c.PublisherID, c.Name, c.ID)
	if err != nil {
		return fmt.Errorf("update campaign: %w", err)
	}
	return nil
}

// DeleteCampaign removes a campaign by ID, first deleting related entities.
func (p *Postgres) DeleteCampaign(id int) error {
	// First delete creatives referencing this campaign
	_, err := p.DB.ExecContext(context.Background(), `DELETE FROM creatives WHERE campaign_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete creatives for campaign: %w", err)
	}

	// Then delete line items referencing this campaign
	_, err = p.DB.ExecContext(context.Background(), `DELETE FROM line_items WHERE campaign_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete line items for campaign: %w", err)
	}

	// Finally delete the campaign
	_, err = p.DB.ExecContext(context.Background(), `DELETE FROM campaigns WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete campaign: %w", err)
	}
	return nil
}

// InsertPlacement inserts a new placement.
func (p *Postgres) InsertPlacement(pl models.Placement) error {
	_, err := p.DB.ExecContext(context.Background(), `INSERT INTO placements (id, publisher_id, width, height, formats) VALUES ($1,$2,$3,$4,$5)`, pl.ID, pl.PublisherID, pl.Width, pl.Height, pq.Array(pl.Formats))
	if err != nil {
		return fmt.Errorf("insert placement: %w", err)
	}
	return nil
}

// UpdatePlacement updates an existing placement.
func (p *Postgres) UpdatePlacement(pl models.Placement) error {
	_, err := p.DB.ExecContext(context.Background(), `UPDATE placements SET publisher_id=$1, width=$2, height=$3, formats=$4 WHERE id=$5`, pl.PublisherID, pl.Width, pl.Height, pq.Array(pl.Formats), pl.ID)
	if err != nil {
		return fmt.Errorf("update placement: %w", err)
	}
	return nil
}

// DeletePlacement removes a placement by ID, first deleting related creatives.
func (p *Postgres) DeletePlacement(id string) error {
	// First delete creatives referencing this placement
	_, err := p.DB.ExecContext(context.Background(), `DELETE FROM creatives WHERE placement_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete creatives for placement: %w", err)
	}

	// Then delete the placement
	_, err = p.DB.ExecContext(context.Background(), `DELETE FROM placements WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete placement: %w", err)
	}
	return nil
}

// InsertLineItem inserts a new line item and returns the generated ID.
func (p *Postgres) InsertLineItem(li *models.LineItem) error {
	kv, _ := json.Marshal(li.KeyValues)
	err := p.DB.QueryRowContext(context.Background(), `INSERT INTO line_items (
        campaign_id, publisher_id, name, start_date, end_date,
        daily_impression_cap, daily_click_cap, pace_type, priority,
        frequency_cap, frequency_window, country, device_type, os, browser,
        active, key_values, cpm, cpc, ecpm, budget_type, budget_amount, spend,
        li_type, endpoint, click_url) VALUES (
        $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26
    ) RETURNING id`,
		li.CampaignID, li.PublisherID, li.Name, li.StartDate, li.EndDate,
		li.DailyImpressionCap, li.DailyClickCap, li.PaceType, li.Priority,
		li.FrequencyCap, int(li.FrequencyWindow.Seconds()), li.Country,
		li.DeviceType, li.OS, li.Browser, li.Active, kv, li.CPM, li.CPC,
		li.ECPM, li.BudgetType, li.BudgetAmount, li.Spend, li.Type, li.Endpoint, li.ClickURL).Scan(&li.ID)
	if err != nil {
		return fmt.Errorf("insert line item: %w", err)
	}
	return nil
}

// UpdateLineItem updates an existing line item.
func (p *Postgres) UpdateLineItem(li models.LineItem) error {
	kv, _ := json.Marshal(li.KeyValues)
	_, err := p.DB.ExecContext(context.Background(), `UPDATE line_items SET
        campaign_id=$1, publisher_id=$2, name=$3, start_date=$4, end_date=$5,
        daily_impression_cap=$6, daily_click_cap=$7, pace_type=$8, priority=$9,
        frequency_cap=$10, frequency_window=$11, country=$12, device_type=$13,
        os=$14, browser=$15, active=$16, key_values=$17, cpm=$18, cpc=$19,
        ecpm=$20, budget_type=$21, budget_amount=$22, spend=$23, li_type=$24,
        endpoint=$25, click_url=$26 WHERE id=$27`,
		li.CampaignID, li.PublisherID, li.Name, li.StartDate, li.EndDate,
		li.DailyImpressionCap, li.DailyClickCap, li.PaceType, li.Priority,
		li.FrequencyCap, int(li.FrequencyWindow.Seconds()), li.Country,
		li.DeviceType, li.OS, li.Browser, li.Active, kv, li.CPM, li.CPC,
		li.ECPM, li.BudgetType, li.BudgetAmount, li.Spend, li.Type,
		li.Endpoint, li.ClickURL, li.ID)
	if err != nil {
		return fmt.Errorf("update line item: %w", err)
	}
	return nil
}

// DeleteLineItem removes a line item by ID, first deleting related creatives.
func (p *Postgres) DeleteLineItem(id int) error {
	// First delete any creatives referencing this line item
	_, err := p.DB.ExecContext(context.Background(), `DELETE FROM creatives WHERE line_item_id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete creatives for line item: %w", err)
	}

	// Then delete the line item
	_, err = p.DB.ExecContext(context.Background(), `DELETE FROM line_items WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete line item: %w", err)
	}
	return nil
}

// InsertCreative inserts a new creative and returns the generated ID.
func (p *Postgres) InsertCreative(c *models.Creative) error {
	var nativeParam interface{}
	if len(c.Native) == 0 || string(c.Native) == "" {
		nativeParam = nil
	} else {
		nativeParam = c.Native
	}

	var bannerParam interface{}
	if len(c.Banner) == 0 || string(c.Banner) == "" {
		bannerParam = nil
	} else {
		bannerParam = c.Banner
	}

	err := p.DB.QueryRowContext(context.Background(), `INSERT INTO creatives (placement_id, line_item_id, campaign_id, publisher_id, html, native, banner, width, height, format, click_url) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, c.PlacementID, c.LineItemID, c.CampaignID, c.PublisherID, c.HTML, nativeParam, bannerParam, c.Width, c.Height, c.Format, c.ClickURL).Scan(&c.ID)
	if err != nil {
		return fmt.Errorf("insert creative: %w", err)
	}
	return nil
}

// UpdateCreative updates an existing creative.
func (p *Postgres) UpdateCreative(c models.Creative) error {
	var nativeParam interface{}
	if len(c.Native) == 0 || string(c.Native) == "" {
		nativeParam = nil
	} else {
		nativeParam = c.Native
	}

	var bannerParam interface{}
	if len(c.Banner) == 0 || string(c.Banner) == "" {
		bannerParam = nil
	} else {
		bannerParam = c.Banner
	}

	_, err := p.DB.ExecContext(context.Background(), `UPDATE creatives SET placement_id=$1, line_item_id=$2, campaign_id=$3, publisher_id=$4, html=$5, native=$6, banner=$7, width=$8, height=$9, format=$10, click_url=$11 WHERE id=$12`, c.PlacementID, c.LineItemID, c.CampaignID, c.PublisherID, c.HTML, nativeParam, bannerParam, c.Width, c.Height, c.Format, c.ClickURL, c.ID)
	if err != nil {
		return fmt.Errorf("update creative: %w", err)
	}
	return nil
}

// DeleteCreative removes a creative by ID.
func (p *Postgres) DeleteCreative(id int) error {
	_, err := p.DB.ExecContext(context.Background(), `DELETE FROM creatives WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete creative: %w", err)
	}
	return nil
}

// UpdateLineItemSpend persists the current spend for a line item.
func (p *Postgres) UpdateLineItemSpend(id int, spend float64) error {
	_, err := p.DB.ExecContext(context.Background(), `UPDATE line_items SET spend=$1 WHERE id=$2`, spend, id)
	if err != nil {
		return fmt.Errorf("update line item spend: %w", err)
	}
	return nil
}
