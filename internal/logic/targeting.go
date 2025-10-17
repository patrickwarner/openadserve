package logic

import (
	"fmt"
	"net"
	"net/http"
	"strings" // Added for Contains

	"github.com/avct/uasurfer"

	"github.com/patrickwarner/openadserve/internal/geoip"
	"github.com/patrickwarner/openadserve/internal/models"
)

// ResolveTargetingFromUA parses a raw User-Agent string into a rich
// TargetingContext using the uasurfer library.
func ResolveTargetingFromUA(uaString string) models.TargetingContext {
	u := uasurfer.Parse(uaString)

	// Device type
	var deviceType string
	switch u.DeviceType {
	case uasurfer.DeviceComputer:
		deviceType = "desktop"
	case uasurfer.DevicePhone:
		deviceType = "mobile"
	case uasurfer.DeviceTablet:
		deviceType = "tablet"
	default:
		deviceType = "other"
	}

	// OS
	osName := fmt.Sprintf("%s %s", u.OS.Platform.String(), u.OS.Name.String())
	v := u.OS.Version
	osVersion := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	fullOS := fmt.Sprintf("%s %s", osName, osVersion)

	// Browser
	browserName := u.Browser.Name.String()
	bv := u.Browser.Version
	browserVersion := fmt.Sprintf("%d.%d.%d", bv.Major, bv.Minor, bv.Patch)
	fullBrowser := fmt.Sprintf("%s %s", browserName, browserVersion)

	// Bot detection
	isBot := u.IsBot()

	return models.TargetingContext{
		DeviceType: deviceType,
		OS:         fullOS,
		Browser:    fullBrowser,
		IsBot:      isBot,
	}
}

// ResolveTargeting parses the UA string and IP address into a TargetingContext.
func ResolveTargeting(g *geoip.GeoIP, uaString, ipString string) models.TargetingContext {
	ctx := ResolveTargetingFromUA(uaString)
	if ip := net.ParseIP(ipString); ip != nil {
		if g != nil {
			ctx.Country = g.Country(ip)
			ctx.Region = g.Region(ip)
		}
	}
	return ctx
}

// MatchesTargeting checks if a Creative's campaign matches the given
// TargetingContext based on device, OS and browser. Empty fields mean a
// wildcard match.
func MatchesTargeting(c models.Creative, ctx models.TargetingContext, dataStore models.AdDataStore) bool {
	li := dataStore.GetLineItem(c.PublisherID, c.LineItemID)

	if li != nil {
		ctxCountry := strings.ToLower(ctx.Country)
		ctxRegion := strings.ToLower(ctx.Region)
		ctxDevice := strings.ToLower(ctx.DeviceType)
		ctxOS := strings.ToLower(ctx.OS)
		ctxBrowser := strings.ToLower(ctx.Browser)

		liCountry := strings.ToLower(li.Country)
		liRegion := strings.ToLower(li.Region)
		liDevice := strings.ToLower(li.DeviceType)
		liOS := strings.ToLower(li.OS)
		liBrowser := strings.ToLower(li.Browser)

		if li.Country != "" && liCountry != ctxCountry {
			return false
		}
		if li.Region != "" && liRegion != ctxRegion {
			return false
		}
		if li.DeviceType != "" && liDevice != ctxDevice {
			return false
		}
		// Logic change: context value (more specific) should contain the line item rule (less specific)
		if li.OS != "" && !strings.Contains(ctxOS, liOS) {
			return false
		}
		// Logic change: context value (more specific) should contain the line item rule (less specific)
		if li.Browser != "" && !strings.Contains(ctxBrowser, liBrowser) {
			return false
		}
		if !MatchesKeyValues(li, ctx) {
			return false
		}
		return true
	}
	// If li is nil (e.g. creative has no line item), it cannot satisfy targeting.
	return false
}

// MatchesKeyValues returns true if all line item key/value pairs are present in the request context.
func MatchesKeyValues(li *models.LineItem, ctx models.TargetingContext) bool {
	if li == nil {
		return true
	}
	if len(li.KeyValues) == 0 {
		return true
	}
	for k, v := range li.KeyValues {
		if ctx.KeyValues == nil {
			return false
		}
		if cv, ok := ctx.KeyValues[k]; !ok || cv != v {
			return false
		}
	}
	return true
}

// ResolveTargetingFromRequest extracts device type and country from HTTP request.
// This is used at impression/click time to get contextual data without storing it in tokens.
func ResolveTargetingFromRequest(r *http.Request, geoIP *geoip.GeoIP) (deviceType, country string) {
	// Get device type from User-Agent
	if ua := r.Header.Get("User-Agent"); ua != "" {
		ctx := ResolveTargetingFromUA(ua)
		deviceType = ctx.DeviceType
	}

	// Get country from IP address
	if geoIP != nil {
		// Try X-Forwarded-For first (handles proxies)
		ipStr := r.Header.Get("X-Forwarded-For")
		if ipStr == "" {
			// Fall back to RemoteAddr
			ipStr = r.RemoteAddr
			// Remove port if present
			if host, _, err := net.SplitHostPort(ipStr); err == nil {
				ipStr = host
			}
		} else {
			// X-Forwarded-For can be comma-separated, take first IP
			if idx := strings.Index(ipStr, ","); idx != -1 {
				ipStr = strings.TrimSpace(ipStr[:idx])
			}
		}

		if ip := net.ParseIP(ipStr); ip != nil {
			country = geoIP.Country(ip)
		}
	}

	return deviceType, country
}
