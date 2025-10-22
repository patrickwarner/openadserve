package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // ClickHouse driver
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/forecasting"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// AdCP Media Buy Protocol request/response types
type GetProductsInput struct {
	PublisherID  int       `json:"publisher_id"`
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	MinBudget    float64   `json:"min_budget,omitempty"`
	BudgetType   string    `json:"budget_type,omitempty"` // CPM, CPC, Flat
	Priority     int       `json:"priority,omitempty"`    // 1-10 priority level
	CPM          float64   `json:"cpm,omitempty"`         // Cost per mille
	CPC          float64   `json:"cpc,omitempty"`         // Cost per click
	PlacementIDs []string  `json:"placement_ids,omitempty"` // Specific placements to forecast
}

type Product struct {
	ID                   string  `json:"id"`
	PlacementID          string  `json:"placement_id"`
	PlacementName        string  `json:"placement_name"`
	Publisher            string  `json:"publisher"`
	Format               string  `json:"format"`
	Width                int     `json:"width"`
	Height               int     `json:"height"`
	AvailableImpressions int64   `json:"available_impressions"`
	MinCPM               float64 `json:"min_cpm,omitempty"`
	EstimatedCTR         float64 `json:"estimated_ctr,omitempty"`
}

type GetProductsOutput struct {
	Products []Product `json:"products"`
}

type CreateMediaBuyInput struct {
	Name        string `json:"name"`
	PublisherID int    `json:"publisher_id"`
	Budget      float64 `json:"budget"`
	BudgetType  string `json:"budget_type"`
	PlacementID string `json:"placement_id"`
}

type CreateMediaBuyOutput struct {
	CampaignID int    `json:"campaign_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}

// AdCP Server holds our dependencies
type AdCPServer struct {
	pg          *db.Postgres
	adDataStore models.AdDataStore
	forecast    *forecasting.Engine
	logger      *zap.Logger
}

// GetProducts implements the AdCP get_products task
func (s *AdCPServer) GetProducts(ctx context.Context, req *mcp.CallToolRequest, input GetProductsInput) (*mcp.CallToolResult, GetProductsOutput, error) {
	// Add overall timeout to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Get all placements and filter by publisher
	allPlacements := s.adDataStore.GetAllPlacements()
	s.logger.Info("Found placements in ad data store", 
		zap.Int("total_placements", len(allPlacements)),
		zap.Int("publisher_id", input.PublisherID))
	
	var products []Product

	for _, placement := range allPlacements {
		s.logger.Debug("Checking placement", 
			zap.String("placement_id", placement.ID),
			zap.Int("placement_publisher_id", placement.PublisherID),
			zap.Int("requested_publisher_id", input.PublisherID))
		if placement.PublisherID != input.PublisherID {
			continue
		}
		s.logger.Info("Including placement for forecasting", 
			zap.String("placement_id", placement.ID),
			zap.Int("publisher_id", input.PublisherID))

		// Determine primary format
		format := "display" // default
		if len(placement.Formats) > 0 {
			format = placement.Formats[0]
		}

		// Default available impressions (since forecasting may not work without ClickHouse)
		availableImpressions := int64(100000) // Default 100k impressions

		// Try to use forecasting engine if available
		if s.forecast != nil {
			// Set budget values for forecasting
			s.logger.Info("Processing budget type", 
				zap.String("placement_id", placement.ID),
				zap.String("input_budget_type", input.BudgetType))
			
			budgetType := input.BudgetType
			if budgetType == "" {
				budgetType = "cpm"
			} else {
				// Convert to lowercase to match model constants
				budgetType = strings.ToLower(budgetType)
			}
			
			s.logger.Info("Converted budget type", 
				zap.String("placement_id", placement.ID),
				zap.String("original", input.BudgetType),
				zap.String("converted", budgetType))
			budget := input.MinBudget
			if budget == 0 {
				budget = 1000.0 // Default $1000 budget
			}
			priority := input.Priority
			if priority == 0 {
				priority = 5 // Default medium priority
			}
			cpm := input.CPM
			if cpm == 0 {
				cpm = 2.0 // Default $2 CPM
			}
			cpc := input.CPC
			if cpc == 0 {
				cpc = 1.0 // Default $1 CPC
			}

			// Use specific placement IDs if provided, otherwise this placement
			placementIDs := input.PlacementIDs
			if len(placementIDs) == 0 {
				placementIDs = []string{placement.ID}
			}

			forecastReq := &models.ForecastRequest{
				PublisherID:  input.PublisherID,
				PlacementIDs: placementIDs,
				StartDate:    input.StartDate,
				EndDate:      input.EndDate,
				BudgetType:   budgetType,
				Budget:       budget,
				Priority:     priority,
				CPM:          cpm,
				CPC:          cpc,
			}

			s.logger.Info("Sending forecast request", 
				zap.String("placement_id", placement.ID),
				zap.Int("publisher_id", forecastReq.PublisherID),
				zap.String("budget_type", forecastReq.BudgetType),
				zap.Float64("budget", forecastReq.Budget),
				zap.Int("priority", forecastReq.Priority),
				zap.Float64("cpm", forecastReq.CPM),
				zap.Float64("cpc", forecastReq.CPC))

			// Add a timeout context to prevent hanging
			forecastCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			if forecastResult, err := s.forecast.Forecast(forecastCtx, forecastReq); err == nil && forecastResult != nil {
				availableImpressions = forecastResult.AvailableImpressions
				s.logger.Info("Forecasting SUCCESS", 
					zap.String("placement_id", placement.ID),
					zap.Int64("available_impressions", availableImpressions),
					zap.Int64("estimated_impressions", forecastResult.EstimatedImpressions),
					zap.Float64("fill_rate", forecastResult.FillRate))
			} else {
				s.logger.Error("Forecasting FAILED", 
					zap.String("placement_id", placement.ID),
					zap.Error(err),
					zap.Int64("using_default", availableImpressions))
			}
		} else {
			s.logger.Warn("Forecasting engine not available", 
				zap.String("placement_id", placement.ID),
				zap.Int64("using_default", availableImpressions))
		}

		product := Product{
			ID:                   fmt.Sprintf("placement_%s", placement.ID),
			PlacementID:          placement.ID,
			PlacementName:        placement.ID, // Use ID as name since Name field doesn't exist
			Publisher:            fmt.Sprintf("publisher_%d", input.PublisherID),
			Format:               format,
			Width:                placement.Width,
			Height:               placement.Height,
			AvailableImpressions: availableImpressions,
		}

		// Set minimum CPM based on budget type
		if strings.ToLower(input.BudgetType) == "cpm" || input.BudgetType == "" {
			product.MinCPM = 1.0 // Default $1 CPM, should come from publisher settings
		}

		products = append(products, product)
	}

	// Ensure we always return a valid array, even if empty
	if products == nil {
		products = []Product{}
	}

	return nil, GetProductsOutput{Products: products}, nil
}

// CreateMediaBuy implements the AdCP create_media_buy task  
func (s *AdCPServer) CreateMediaBuy(ctx context.Context, req *mcp.CallToolRequest, input CreateMediaBuyInput) (*mcp.CallToolResult, CreateMediaBuyOutput, error) {
	// Create campaign using database
	campaign := &models.Campaign{
		Name:        input.Name,
		PublisherID: input.PublisherID,
	}

	if err := s.pg.InsertCampaign(campaign); err != nil {
		return nil, CreateMediaBuyOutput{}, fmt.Errorf("failed to create campaign: %w", err)
	}

	// Create a basic line item
	lineItem := &models.LineItem{
		CampaignID:   campaign.ID,
		PublisherID:  input.PublisherID,
		Name:         fmt.Sprintf("%s - Line Item", input.Name),
		Type:         models.LineItemTypeDirect,
		Priority:     models.PriorityMedium,
		BudgetAmount: input.Budget,
		BudgetType:   input.BudgetType,
		StartDate:    time.Now(),
		EndDate:      time.Now().AddDate(0, 1, 0), // Default 1 month
		PaceType:     "even",
		Active:       true,
	}

	// Set CPM or CPC based on budget type
	if strings.ToLower(input.BudgetType) == "cpm" {
		lineItem.CPM = input.Budget / 1000 // Convert to CPM rate
		lineItem.ECPM = lineItem.CPM
	} else if strings.ToLower(input.BudgetType) == "cpc" {
		lineItem.CPC = 1.0 // Default $1 CPC
		lineItem.ECPM = lineItem.CPC * 0.02 * 1000 // Assume 2% CTR
	}

	if err := s.pg.InsertLineItem(lineItem); err != nil {
		return nil, CreateMediaBuyOutput{}, fmt.Errorf("failed to create line item: %w", err)
	}

	return nil, CreateMediaBuyOutput{
		CampaignID: campaign.ID,
		Status:     "active",
		Message:    fmt.Sprintf("Successfully created campaign '%s'", input.Name),
	}, nil
}

func main() {
	// Initialize logger for MCP server - use stderr to avoid stdio conflicts
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	cfg.OutputPaths = []string{"stderr"}      // Force stderr output
	cfg.ErrorOutputPaths = []string{"stderr"} // Force stderr for errors
	
	// Use same encoder config as observability package for consistency
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.NameKey = "logger"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.MessageKey = "msg"
	cfg.EncoderConfig.StacktraceKey = "stacktrace"
	
	logger, err := cfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	
	// Add service name as a permanent field for consistency
	logger = logger.Named("openadserve-mcp").With(zap.String("service", "openadserve-mcp"))
	
	logger.Info("Starting OpenAdServe MCP Server - NEW VERSION")

	// Initialize database connections
	postgresURL := os.Getenv("POSTGRES_DSN")
	if postgresURL == "" {
		logger.Fatal("POSTGRES_DSN environment variable is required")
	}

	// Initialize database connection
	pg, err := db.InitPostgres(postgresURL, 10, 5, 30*time.Minute)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer pg.Close()
	logger.Info("Connected to PostgreSQL")

	// Initialize Redis connection
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer redisClient.Close()
	logger.Info("Connected to Redis", zap.String("addr", redisAddr))

	// Initialize ClickHouse connection (for forecasting)
	clickhouseDSN := os.Getenv("CLICKHOUSE_DSN")
	if clickhouseDSN == "" {
		clickhouseDSN = "clickhouse://default:@localhost:9000/default"
	}
	
	// Properly initialize ClickHouse
	clickhouseDB, err := sql.Open("clickhouse", clickhouseDSN)
	if err != nil {
		logger.Warn("Failed to connect to ClickHouse, forecasting will use defaults", zap.Error(err))
		clickhouseDB = nil
	} else {
		clickhouseDB.SetMaxOpenConns(25)
		if err := clickhouseDB.PingContext(context.Background()); err != nil {
			logger.Warn("ClickHouse ping failed, forecasting will use defaults", zap.Error(err))
			clickhouseDB.Close()
			clickhouseDB = nil
		} else {
			logger.Info("ClickHouse connected successfully for forecasting")
			defer clickhouseDB.Close()
		}
	}

	// Initialize ad data store
	adDataStore := models.NewInMemoryAdDataStore()

	// Load data from Postgres
	logger.Info("Loading data from Postgres")
	items, err := pg.LoadLineItems()
	if err != nil {
		logger.Fatal("Failed to load line items", zap.Error(err))
	}
	campaigns, err := pg.LoadCampaigns()
	if err != nil {
		logger.Fatal("Failed to load campaigns", zap.Error(err))
	}
	publishers, err := pg.LoadPublishers()
	if err != nil {
		logger.Fatal("Failed to load publishers", zap.Error(err))
	}
	placements, err := pg.LoadPlacements()
	if err != nil {
		logger.Fatal("Failed to load placements", zap.Error(err))
	}
	
	logger.Info("Loaded data from Postgres", 
		zap.Int("line_items", len(items)),
		zap.Int("campaigns", len(campaigns)),
		zap.Int("publishers", len(publishers)),
		zap.Int("placements", len(placements)))
	
	if err := adDataStore.ReloadAll(items, campaigns, publishers, placements); err != nil {
		logger.Fatal("Failed to populate ad data store", zap.Error(err))
	}
	
	logger.Info("Ad data store populated successfully")

	// Initialize forecasting engine
	forecast := forecasting.NewEngine(clickhouseDB, redisClient, adDataStore, logger)

	// Create our AdCP server
	adcpServer := &AdCPServer{
		pg:          pg,
		adDataStore: adDataStore,
		forecast:    forecast,
		logger:      logger,
	}

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "openadserve",
		Version: "1.0.0",
	}, nil)

	// Add AdCP tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_products",
		Description: "Discover available advertising inventory using AdCP Media Buy Protocol",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"publisher_id": map[string]interface{}{
					"type":        "integer",
					"description": "Publisher ID to get inventory for",
				},
				"start_date": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "Campaign start date",
				},
				"end_date": map[string]interface{}{
					"type":        "string",
					"format":      "date-time",
					"description": "Campaign end date",
				},
				"budget_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"cpm", "cpc", "flat"},
					"description": "Budget type (optional, defaults to cpm)",
				},
				"min_budget": map[string]interface{}{
					"type":        "number",
					"description": "Minimum budget for forecasting (optional, defaults to $1000)",
				},
				"priority": map[string]interface{}{
					"type":        "integer",
					"minimum":     1,
					"maximum":     10,
					"description": "Campaign priority level 1-10 (optional, defaults to 5)",
				},
				"cpm": map[string]interface{}{
					"type":        "number",
					"description": "Cost per mille for CPM campaigns (optional, defaults to $2.00)",
				},
				"cpc": map[string]interface{}{
					"type":        "number",
					"description": "Cost per click for CPC campaigns (optional, defaults to $1.00)",
				},
				"placement_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Specific placement IDs to forecast (optional, forecasts all placements if not provided)",
				},
			},
			"required": []string{"publisher_id", "start_date", "end_date"},
		},
	}, adcpServer.GetProducts)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_media_buy",
		Description: "Create a new advertising campaign using AdCP Media Buy Protocol",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Campaign name",
				},
				"publisher_id": map[string]interface{}{
					"type":        "integer",
					"description": "Publisher ID",
				},
				"budget": map[string]interface{}{
					"type":        "number",
					"description": "Campaign budget",
				},
				"budget_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"cpm", "cpc", "flat"},
					"description": "Budget type",
				},
				"placement_id": map[string]interface{}{
					"type":        "string",
					"description": "Placement ID to target",
				},
			},
			"required": []string{"name", "publisher_id", "budget", "budget_type", "placement_id"},
		},
	}, adcpServer.CreateMediaBuy)

	// Run the MCP server with logging transport for debugging
	stdioTransport := &mcp.StdioTransport{}
	
	// Add logging transport to debug MCP communication
	var logBuffer bytes.Buffer
	loggingTransport := &mcp.LoggingTransport{
		Transport: stdioTransport,
		Writer:    &logBuffer,
	}
	
	logger.Info("MCP Server running via stdio with logging enabled")
	
	if err := server.Run(context.Background(), loggingTransport); err != nil {
		logger.Fatal("Server error", zap.Error(err), zap.String("mcp_logs", logBuffer.String()))
	}
}