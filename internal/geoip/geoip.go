package geoip

import (
	"encoding/json"
	"net"
	"os"

	"github.com/oschwald/geoip2-golang"
)

// GeoIP provides country lookup using a MaxMind DB or a JSON fallback.
type GeoIP struct {
	db       *geoip2.Reader
	fallback []record
}

type record struct {
	net     *net.IPNet
	country string
	region  string
}

// Init opens the GeoIP2 database located at path. It returns a GeoIP instance.
// The returned error indicates problems opening the file.
func Init(path string) (*GeoIP, error) {
	g := &GeoIP{}
	db, err := geoip2.Open(path)
	if err == nil {
		g.db = db
		return g, nil
	}

	data, jerr := os.ReadFile(path)
	if jerr != nil {
		return nil, err
	}
	var entries []struct {
		Net     string `json:"net"`
		Country string `json:"country"`
		Region  string `json:"region"`
	}
	if jerr = json.Unmarshal(data, &entries); jerr != nil {
		return nil, err
	}
	for _, e := range entries {
		if _, n, perr := net.ParseCIDR(e.Net); perr == nil {
			g.fallback = append(g.fallback, record{net: n, country: e.Country, region: e.Region})
		}
	}
	return g, nil
}

// Country returns the ISO country code for the given IP. If the IP is not found
// in the database or the database hasn't been initialised, an empty string is returned.
func (g *GeoIP) Country(ip net.IP) string {
	if g == nil {
		return ""
	}
	if g.db != nil {
		rec, err := g.db.Country(ip)
		if err == nil {
			return rec.Country.IsoCode
		}
	}
	for _, r := range g.fallback {
		if r.net.Contains(ip) {
			return r.country
		}
	}
	return ""
}

// Region returns the region or subdivision code for the given IP. If the IP
// is not found or the database lacks region data, an empty string is returned.
func (g *GeoIP) Region(ip net.IP) string {
	if g == nil {
		return ""
	}
	if g.db != nil {
		rec, err := g.db.City(ip)
		if err == nil && len(rec.Subdivisions) > 0 {
			return rec.Subdivisions[0].IsoCode
		}
	}
	for _, r := range g.fallback {
		if r.net.Contains(ip) {
			return r.region
		}
	}
	return ""
}

// Close releases resources associated with the database.
func (g *GeoIP) Close() error {
	if g != nil && g.db != nil {
		return g.db.Close()
	}
	return nil
}
