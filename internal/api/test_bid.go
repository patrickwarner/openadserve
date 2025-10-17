package api

import (
	"encoding/json"
	"net/http"
)

// TestBidHandler returns a fixed OpenRTB bid response. It can be used as a
// stand-in for a header bidding endpoint when testing programmatic line items.
func (s *Server) TestBidHandler(w http.ResponseWriter, r *http.Request) {
	// Read and discard the request body to mimic an OpenRTB endpoint.
	var req map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&req)
	_ = r.Body.Close()

	resp := map[string]interface{}{
		"seatbid": []map[string]interface{}{
			{
				"bid": []map[string]interface{}{
					{
						"price": 1.75,
						"adm":   "<div>Programmatic Test Creative</div>",
					},
				},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
