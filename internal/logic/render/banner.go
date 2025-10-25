package render

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// BannerData represents the structure of banner ad JSON
type BannerData struct {
	Image  string       `json:"image"`
	Alt    string       `json:"alt"`
	Images []BannerSize `json:"images"`
}

// BannerSize represents a responsive image variant
type BannerSize struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// ComposeBannerHTML converts banner JSON into HTML markup for client rendering.
// This server-side composition reduces client-side processing and ensures consistent rendering.
func ComposeBannerHTML(bannerJSON json.RawMessage) string {
	if len(bannerJSON) == 0 {
		return ""
	}

	var banner BannerData
	if err := json.Unmarshal(bannerJSON, &banner); err != nil {
		// If parsing fails, return empty - creative won't render
		return ""
	}

	// Build img tag with responsive srcset
	var imgParts []string

	// Main image source (required)
	if banner.Image != "" {
		imgParts = append(imgParts, fmt.Sprintf(`src="%s"`, html.EscapeString(banner.Image)))
	} else if len(banner.Images) > 0 {
		// Fallback to first responsive image if main image missing
		imgParts = append(imgParts, fmt.Sprintf(`src="%s"`, html.EscapeString(banner.Images[0].URL)))
	} else {
		// No image URL available
		return ""
	}

	// Alt text (important for accessibility)
	altText := banner.Alt
	if altText == "" {
		altText = "Advertisement"
	}
	imgParts = append(imgParts, fmt.Sprintf(`alt="%s"`, html.EscapeString(altText)))

	// Build srcset for responsive images
	if len(banner.Images) > 0 {
		var srcsetParts []string
		for _, img := range banner.Images {
			if img.URL != "" && img.Width > 0 {
				srcsetParts = append(srcsetParts, fmt.Sprintf("%s %dw", html.EscapeString(img.URL), img.Width))
			}
		}
		if len(srcsetParts) > 0 {
			imgParts = append(imgParts, fmt.Sprintf(`srcset="%s"`, strings.Join(srcsetParts, ", ")))
		}
	}

	// Add styling for responsive display
	// These styles ensure the image fits within its container while maintaining aspect ratio
	imgParts = append(imgParts, `style="max-width:100%;max-height:100%;width:auto;height:auto;display:block;cursor:pointer;"`)

	// Compose final HTML
	return fmt.Sprintf("<img %s>", strings.Join(imgParts, " "))
}
