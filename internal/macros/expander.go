package macros

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// MacroExpander handles macro expansion in click URLs with observability
type MacroExpander struct {
	logger       *zap.Logger
	expansions   map[string]ExpansionFunc
	expansionsMu sync.RWMutex
	strictMode   bool // If true, any macro expansion failure causes the entire operation to fail

	// Metrics
	expansionCounter  *prometheus.CounterVec
	expansionDuration prometheus.Histogram
	failureCounter    *prometheus.CounterVec
}

// ExpansionFunc defines the signature for macro expansion functions
type ExpansionFunc func(ctx *ExpansionContext) (string, error)

// ExpansionContext contains all data available for macro expansion
type ExpansionContext struct {
	// Request context
	RequestID    string
	ImpressionID string
	Timestamp    time.Time

	// Ad context
	CreativeID  int32
	LineItemID  int32
	CampaignID  int32
	PublisherID int32
	PlacementID string

	// Custom parameters
	CustomParams map[string]string
}

// NewMacroExpander creates a new macro expander with default macros
func NewMacroExpander(logger *zap.Logger) *MacroExpander {
	return NewMacroExpanderWithMode(logger, false) // Default to lenient mode
}

// NewMacroExpanderWithMode creates a new macro expander with configurable strict/lenient mode
func NewMacroExpanderWithMode(logger *zap.Logger, strictMode bool) *MacroExpander {
	expander := &MacroExpander{
		logger:     logger,
		expansions: make(map[string]ExpansionFunc),
		strictMode: strictMode,

		// Use global registry for production observability
		expansionCounter: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "macro_expansions_total",
				Help: "Total number of macro expansions performed",
			},
			[]string{"macro", "success"},
		),
		expansionDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "macro_expansion_duration_seconds",
				Help:    "Time taken to expand all macros in a URL",
				Buckets: prometheus.DefBuckets,
			},
		),
		failureCounter: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "macro_expansion_failures_total",
				Help: "Total number of macro expansion failures",
			},
			[]string{"macro", "error_type"},
		),
	}

	// Register default macros
	expander.registerDefaultMacros()

	return expander
}

// NewMacroExpanderForTesting creates a new macro expander with a custom registry for testing
func NewMacroExpanderForTesting(logger *zap.Logger, strictMode bool) *MacroExpander {
	// Use a custom registry to avoid conflicts in tests
	registry := prometheus.NewRegistry()
	factory := promauto.With(registry)

	expander := &MacroExpander{
		logger:     logger,
		expansions: make(map[string]ExpansionFunc),
		strictMode: strictMode,

		// Initialize metrics with custom registry for testing
		expansionCounter: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "macro_expansions_total",
				Help: "Total number of macro expansions performed",
			},
			[]string{"macro", "success"},
		),
		expansionDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "macro_expansion_duration_seconds",
				Help:    "Time taken to expand all macros in a URL",
				Buckets: prometheus.DefBuckets,
			},
		),
		failureCounter: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "macro_expansion_failures_total",
				Help: "Total number of macro expansion failures",
			},
			[]string{"macro", "error_type"},
		),
	}

	// Register default macros
	expander.registerDefaultMacros()

	return expander
}

// SetStrictMode enables or disables strict macro expansion mode
func (e *MacroExpander) SetStrictMode(strict bool) {
	e.strictMode = strict
}

// ExpandURL expands all macros in the given URL
func (e *MacroExpander) ExpandURL(rawURL string, ctx *ExpansionContext) (string, error) {
	start := time.Now()
	defer func() {
		e.expansionDuration.Observe(time.Since(start).Seconds())
	}()

	if rawURL == "" {
		return "", nil
	}

	// Parse URL to validate it
	_, err := url.Parse(rawURL)
	if err != nil {
		e.logger.Error("Failed to parse URL for macro expansion",
			zap.String("url", rawURL),
			zap.Error(err))
		return rawURL, err
	}

	// First, handle custom parameters {CUSTOM.key}
	expanded := e.expandCustomParams(rawURL, ctx)

	// Optimized macro expansion using strings.Replacer for better performance
	expanded, macrosFound, err := e.expandStandardMacros(expanded, ctx)
	if err != nil {
		// In production (non-strict mode), log error but continue with partially expanded URL
		if !e.strictMode {
			e.logger.Warn("Macro expansion completed with errors, continuing with partial expansion",
				zap.String("original_url", rawURL),
				zap.String("partial_url", expanded),
				zap.Error(err))
		} else {
			// In strict mode (testing), return the error
			return "", err
		}
	}

	if macrosFound > 0 {
		e.logger.Debug("Expanded macros in URL",
			zap.String("original_url", rawURL),
			zap.String("expanded_url", expanded),
			zap.Int("macros_found", macrosFound))
	}

	return expanded, nil
}

// expandStandardMacros uses an optimized approach for multiple macro replacements
func (e *MacroExpander) expandStandardMacros(rawURL string, ctx *ExpansionContext) (string, int, error) {
	e.expansionsMu.RLock()
	defer e.expansionsMu.RUnlock()

	// Pre-scan URL to identify which macros are present to avoid unnecessary work
	var foundMacros []string
	var replacements []string

	for macro := range e.expansions {
		placeholder := "{" + macro + "}"
		if strings.Contains(rawURL, placeholder) {
			foundMacros = append(foundMacros, macro)
		}
	}

	// If no macros found, return early
	if len(foundMacros) == 0 {
		return rawURL, 0, nil
	}

	// Build replacement pairs for strings.Replacer
	for _, macro := range foundMacros {
		placeholder := "{" + macro + "}"
		expansionFunc := e.expansions[macro]

		value, err := expansionFunc(ctx)
		if err != nil {
			e.expansionCounter.WithLabelValues(macro, "false").Inc()
			e.failureCounter.WithLabelValues(macro, "expansion_error").Inc()
			e.logger.Error("Failed to expand macro",
				zap.String("macro", macro),
				zap.String("url", rawURL),
				zap.Error(err))

			// In strict mode, return error immediately on any expansion failure
			if e.strictMode {
				return "", 0, fmt.Errorf("macro expansion failed in strict mode for macro '%s': %w", macro, err)
			}
			continue
		}

		// URL encode the expanded value
		encodedValue := url.QueryEscape(value)
		replacements = append(replacements, placeholder, encodedValue)

		e.expansionCounter.WithLabelValues(macro, "true").Inc()
	}

	// Use strings.Replacer for efficient multiple replacements
	if len(replacements) > 0 {
		replacer := strings.NewReplacer(replacements...)
		return replacer.Replace(rawURL), len(foundMacros), nil
	}

	return rawURL, 0, nil
}

// RegisterMacro adds a custom macro expansion function
func (e *MacroExpander) RegisterMacro(name string, expansionFunc ExpansionFunc) error {
	if name == "" {
		return fmt.Errorf("macro name cannot be empty")
	}

	if expansionFunc == nil {
		return fmt.Errorf("expansion function cannot be nil")
	}

	e.expansionsMu.Lock()
	defer e.expansionsMu.Unlock()

	e.expansions[name] = expansionFunc

	e.logger.Info("Registered custom macro",
		zap.String("macro", name))

	return nil
}

// GetRegisteredMacros returns a list of all registered macro names
func (e *MacroExpander) GetRegisteredMacros() []string {
	e.expansionsMu.RLock()
	defer e.expansionsMu.RUnlock()

	macros := make([]string, 0, len(e.expansions))
	for name := range e.expansions {
		macros = append(macros, name)
	}

	return macros
}

// registerDefaultMacros registers commonly used industry-standard macros
func (e *MacroExpander) registerDefaultMacros() {
	// IAB OpenRTB standard macros
	e.expansions["AUCTION_ID"] = func(ctx *ExpansionContext) (string, error) {
		return ctx.RequestID, nil
	}

	e.expansions["AUCTION_IMP_ID"] = func(ctx *ExpansionContext) (string, error) {
		return ctx.ImpressionID, nil
	}

	// Creative and campaign identifiers
	e.expansions["CREATIVE_ID"] = func(ctx *ExpansionContext) (string, error) {
		return fmt.Sprintf("%d", ctx.CreativeID), nil
	}

	e.expansions["LINE_ITEM_ID"] = func(ctx *ExpansionContext) (string, error) {
		return fmt.Sprintf("%d", ctx.LineItemID), nil
	}

	e.expansions["CAMPAIGN_ID"] = func(ctx *ExpansionContext) (string, error) {
		return fmt.Sprintf("%d", ctx.CampaignID), nil
	}

	e.expansions["PUBLISHER_ID"] = func(ctx *ExpansionContext) (string, error) {
		return fmt.Sprintf("%d", ctx.PublisherID), nil
	}

	e.expansions["PLACEMENT_ID"] = func(ctx *ExpansionContext) (string, error) {
		return ctx.PlacementID, nil
	}

	// Timestamp macros
	e.expansions["TIMESTAMP"] = func(ctx *ExpansionContext) (string, error) {
		return fmt.Sprintf("%d", ctx.Timestamp.Unix()), nil
	}

	e.expansions["TIMESTAMP_MS"] = func(ctx *ExpansionContext) (string, error) {
		return fmt.Sprintf("%d", ctx.Timestamp.UnixMilli()), nil
	}

	e.expansions["ISO_TIMESTAMP"] = func(ctx *ExpansionContext) (string, error) {
		return ctx.Timestamp.Format(time.RFC3339), nil
	}

	// Random values for cache busting
	e.expansions["RANDOM"] = func(ctx *ExpansionContext) (string, error) {
		return fmt.Sprintf("%d", time.Now().UnixNano()), nil
	}

	e.expansions["UUID"] = func(ctx *ExpansionContext) (string, error) {
		return uuid.New().String(), nil
	}

	// Custom parameter expansion
	e.expansions["CUSTOM"] = func(ctx *ExpansionContext) (string, error) {
		// This is a special case - CUSTOM.key will be handled separately
		return "", fmt.Errorf("CUSTOM macro requires a parameter key")
	}
}

// ExpandCustomParameter expands custom parameters like {CUSTOM.key}
func (e *MacroExpander) ExpandCustomParameter(key string, ctx *ExpansionContext) (string, error) {
	if ctx.CustomParams == nil {
		return "", fmt.Errorf("no custom parameters available")
	}

	value, exists := ctx.CustomParams[key]
	if !exists {
		return "", fmt.Errorf("custom parameter '%s' not found", key)
	}

	return value, nil
}

// expandCustomParams expands {CUSTOM.key} patterns in the URL
func (e *MacroExpander) expandCustomParams(rawURL string, ctx *ExpansionContext) string {
	if ctx.CustomParams == nil {
		return rawURL
	}

	expanded := rawURL
	for key, value := range ctx.CustomParams {
		placeholder := "{CUSTOM." + key + "}"
		if strings.Contains(expanded, placeholder) {
			encodedValue := url.QueryEscape(value)
			expanded = strings.ReplaceAll(expanded, placeholder, encodedValue)
		}
	}

	return expanded
}

// ValidateURL checks if a URL contains only supported macros
func (e *MacroExpander) ValidateURL(rawURL string) []string {
	var unsupportedMacros []string

	// Find all macro placeholders in the URL
	macroStart := 0
	for {
		start := strings.Index(rawURL[macroStart:], "{")
		if start == -1 {
			break
		}
		start += macroStart

		end := strings.Index(rawURL[start:], "}")
		if end == -1 {
			break
		}
		end += start

		macro := rawURL[start+1 : end]

		// Handle CUSTOM.key pattern
		if strings.HasPrefix(macro, "CUSTOM.") {
			macroStart = end + 1
			continue
		}

		// Check if macro is supported
		e.expansionsMu.RLock()
		_, supported := e.expansions[macro]
		e.expansionsMu.RUnlock()

		if !supported {
			unsupportedMacros = append(unsupportedMacros, macro)
		}

		macroStart = end + 1
	}

	return unsupportedMacros
}
