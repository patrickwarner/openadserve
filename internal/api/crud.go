package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// helper function to write JSON response
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ===== Publishers =====

func (s *Server) ListPublishers(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	pubs := s.AdDataStore.GetAllPublishers()
	writeJSON(w, pubs)
}

func (s *Server) CreatePublisher(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	var pub models.Publisher
	if err := json.NewDecoder(r.Body).Decode(&pub); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// First persist to PostgreSQL to get the ID
	if s.PG != nil {
		if err := s.PG.InsertPublisher(&pub); err != nil {
			s.Logger.Error("insert publisher to postgres", zap.Error(err))
			http.Error(w, "failed to persist publisher", http.StatusInternalServerError)
			return
		}
	}

	// Then insert into data store with the ID from PostgreSQL
	if err := s.AdDataStore.InsertPublisher(&pub); err != nil {
		s.Logger.Error("insert publisher to data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.notifyUpdate("publisher", "create", pub.ID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, pub)
}

func (s *Server) UpdatePublisher(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var pub models.Publisher
	if err := json.NewDecoder(r.Body).Decode(&pub); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	pub.ID = id

	// Update in data store
	if err := s.AdDataStore.UpdatePublisher(pub); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "publisher not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("update publisher in data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also update in PostgreSQL for persistence
	if s.PG != nil {
		if err := s.PG.UpdatePublisher(pub); err != nil {
			s.Logger.Error("update publisher in postgres", zap.Error(err))
			// Don't fail the request, data store is the source of truth
		}
	}

	s.notifyUpdate("publisher", "update", id)
	writeJSON(w, pub)
}

func (s *Server) DeletePublisher(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Delete from data store
	if err := s.AdDataStore.DeletePublisher(id); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "publisher not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("delete publisher from data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also delete from PostgreSQL for persistence
	if s.PG != nil {
		if err := s.PG.DeletePublisher(id); err != nil {
			s.Logger.Error("delete publisher from postgres", zap.Error(err))
			// Don't fail the request, data store is the source of truth
		}
	}

	s.notifyUpdate("publisher", "delete", id)
	w.WriteHeader(http.StatusNoContent)
}

// ===== Campaigns =====

func (s *Server) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	cs := s.AdDataStore.GetAllCampaigns()
	writeJSON(w, cs)
}

func (s *Server) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	var c models.Campaign
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// First persist to PostgreSQL to get the ID
	if s.PG != nil {
		if err := s.PG.InsertCampaign(&c); err != nil {
			s.Logger.Error("insert campaign to postgres", zap.Error(err))
			http.Error(w, "failed to persist campaign", http.StatusInternalServerError)
			return
		}
	}

	// Then insert into data store with the ID from PostgreSQL
	if err := s.AdDataStore.InsertCampaign(&c); err != nil {
		s.Logger.Error("insert campaign to data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.notifyUpdate("campaign", "create", c.ID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, c)
}

func (s *Server) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var c models.Campaign
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	c.ID = id

	// Update in data store
	if err := s.AdDataStore.UpdateCampaign(c); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "campaign not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("update campaign in data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also update in PostgreSQL
	if s.PG != nil {
		if err := s.PG.UpdateCampaign(c); err != nil {
			s.Logger.Error("update campaign in postgres", zap.Error(err))
		}
	}

	s.notifyUpdate("campaign", "update", id)
	writeJSON(w, c)
}

func (s *Server) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Delete from data store
	if err := s.AdDataStore.DeleteCampaign(id); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "campaign not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("delete campaign from data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also delete from PostgreSQL
	if s.PG != nil {
		if err := s.PG.DeleteCampaign(id); err != nil {
			s.Logger.Error("delete campaign from postgres", zap.Error(err))
		}
	}

	s.notifyUpdate("campaign", "delete", id)
	w.WriteHeader(http.StatusNoContent)
}

// ===== Placements =====

func (s *Server) ListPlacements(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	ps := s.AdDataStore.GetAllPlacements()
	writeJSON(w, ps)
}

func (s *Server) CreatePlacement(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	var p models.Placement
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Insert into data store
	if err := s.AdDataStore.InsertPlacement(p); err != nil {
		s.Logger.Error("insert placement to data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also persist to PostgreSQL
	if s.PG != nil {
		if err := s.PG.InsertPlacement(p); err != nil {
			s.Logger.Error("insert placement to postgres", zap.Error(err))
		}
	}

	s.notifyUpdate("placement", "create", p.ID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, p)
}

func (s *Server) UpdatePlacement(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	id := mux.Vars(r)["id"]
	var p models.Placement
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	p.ID = id

	// Update in data store
	if err := s.AdDataStore.UpdatePlacement(p); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "placement not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("update placement in data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also update in PostgreSQL
	if s.PG != nil {
		if err := s.PG.UpdatePlacement(p); err != nil {
			s.Logger.Error("update placement in postgres", zap.Error(err))
		}
	}

	s.notifyUpdate("placement", "update", id)
	writeJSON(w, p)
}

func (s *Server) DeletePlacement(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	id := mux.Vars(r)["id"]

	// Delete from data store
	if err := s.AdDataStore.DeletePlacement(id); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "placement not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("delete placement from data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also delete from PostgreSQL
	if s.PG != nil {
		if err := s.PG.DeletePlacement(id); err != nil {
			s.Logger.Error("delete placement from postgres", zap.Error(err))
		}
	}

	s.notifyUpdate("placement", "delete", id)
	w.WriteHeader(http.StatusNoContent)
}

// ===== Line Items =====

func (s *Server) ListLineItems(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	items := s.AdDataStore.GetAllLineItems()
	writeJSON(w, items)
}

func (s *Server) CreateLineItem(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	var li models.LineItem
	if err := json.NewDecoder(r.Body).Decode(&li); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// First persist to PostgreSQL to get the ID
	if s.PG != nil {
		if err := s.PG.InsertLineItem(&li); err != nil {
			s.Logger.Error("insert line item to postgres", zap.Error(err))
			http.Error(w, "failed to persist line item", http.StatusInternalServerError)
			return
		}
	}

	// Then insert into data store with the ID from PostgreSQL
	if err := s.AdDataStore.InsertLineItem(&li); err != nil {
		s.Logger.Error("insert line item to data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.notifyUpdate("line_item", "create", li.ID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, li)
}

func (s *Server) UpdateLineItem(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var li models.LineItem
	if err := json.NewDecoder(r.Body).Decode(&li); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	li.ID = id

	// Update in data store
	if err := s.AdDataStore.UpdateLineItem(li); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "line item not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("update line item in data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also update in PostgreSQL
	if s.PG != nil {
		if err := s.PG.UpdateLineItem(li); err != nil {
			s.Logger.Error("update line item in postgres", zap.Error(err))
		}
	}

	s.notifyUpdate("line_item", "update", id)
	writeJSON(w, li)
}

func (s *Server) DeleteLineItem(w http.ResponseWriter, r *http.Request) {
	if s.AdDataStore == nil {
		http.Error(w, "data store unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// Delete from data store
	if err := s.AdDataStore.DeleteLineItem(id); err != nil {
		if err == models.ErrNotFound {
			http.Error(w, "line item not found", http.StatusNotFound)
			return
		}
		s.Logger.Error("delete line item from data store", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Also delete from PostgreSQL
	if s.PG != nil {
		if err := s.PG.DeleteLineItem(id); err != nil {
			s.Logger.Error("delete line item from postgres", zap.Error(err))
		}
	}

	s.notifyUpdate("line_item", "delete", id)
	w.WriteHeader(http.StatusNoContent)
}

// ===== Creatives =====

func (s *Server) ListCreatives(w http.ResponseWriter, r *http.Request) {
	if s.PG == nil {
		http.Error(w, "postgres unavailable", http.StatusInternalServerError)
		return
	}
	cs, err := s.PG.LoadCreatives()
	if err != nil {
		s.Logger.Error("load creatives", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, cs)
}

func (s *Server) CreateCreative(w http.ResponseWriter, r *http.Request) {
	if s.PG == nil {
		http.Error(w, "postgres unavailable", http.StatusInternalServerError)
		return
	}
	var c models.Creative
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Auto-populate campaign and publisher from line item
	if c.LineItemID != 0 && s.AdDataStore != nil {
		lineItem := s.AdDataStore.GetLineItemByID(c.LineItemID)
		if lineItem == nil {
			http.Error(w, "line item not found", http.StatusBadRequest)
			return
		}
		c.CampaignID = lineItem.CampaignID
		c.PublisherID = lineItem.PublisherID
	}

	// Auto-populate width and height from placement
	if c.PlacementID != "" && s.AdDataStore != nil {
		placement := s.AdDataStore.GetPlacement(c.PlacementID)
		if placement == nil {
			http.Error(w, "placement not found", http.StatusBadRequest)
			return
		}
		c.Width = placement.Width
		c.Height = placement.Height
	}

	// Note: Creatives are currently only stored in PostgreSQL
	// TODO: Add creative support to AdDataStore interface for full consistency
	if err := s.PG.InsertCreative(&c); err != nil {
		s.Logger.Error("insert creative", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.notifyUpdate("creative", "create", c.ID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, c)
}

func (s *Server) UpdateCreative(w http.ResponseWriter, r *http.Request) {
	if s.PG == nil {
		http.Error(w, "postgres unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var c models.Creative
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	c.ID = id
	if err := s.PG.UpdateCreative(c); err != nil {
		s.Logger.Error("update creative", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.notifyUpdate("creative", "update", id)
	writeJSON(w, c)
}

func (s *Server) DeleteCreative(w http.ResponseWriter, r *http.Request) {
	if s.PG == nil {
		http.Error(w, "postgres unavailable", http.StatusInternalServerError)
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.PG.DeleteCreative(id); err != nil {
		s.Logger.Error("delete creative", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.notifyUpdate("creative", "delete", id)
	w.WriteHeader(http.StatusNoContent)
}
