package models

import (
	"errors"
	"sync/atomic"
)

// ErrNotFound is returned when an entity is not found in the data store
var ErrNotFound = errors.New("entity not found")

// AdDataStore provides thread-safe access to ad server data without global variables.
// It encapsulates line items, campaigns, and publishers with atomic update capabilities.
type AdDataStore interface {
	// Read operations (hot path)
	GetLineItem(publisherID, lineItemID int) *LineItem
	GetLineItemsByPublisher(publisherID int) []LineItem
	GetCampaign(campaignID int) *Campaign
	GetPublisher(publisherID int) *Publisher
	GetPlacement(placementID string) *Placement
	GetLineItemByID(lineItemID int) *LineItem // For backward compatibility

	// Iteration methods
	GetAllPublishers() []Publisher
	GetAllPublisherIDs() []int
	GetAllCampaigns() []Campaign
	GetAllLineItems() []LineItem
	GetAllPlacements() []Placement

	// Write operations (reload path)
	SetLineItems(items []LineItem) error
	SetLineItemsForPublisher(publisherID int, items []LineItem) error
	SetCampaigns(campaigns []Campaign) error
	SetPublishers(publishers []Publisher) error
	SetPlacements(placements []Placement) error

	// Atomic bulk operations
	ReloadAll(lineItems []LineItem, campaigns []Campaign, publishers []Publisher, placements []Placement) error

	// Maintenance operations
	UpdateLineItemECPM(publisherID, lineItemID int, ecpm float64) error
	// UpdateLineItemsECPM updates multiple line items' eCPM values in a single snapshot swap.
	UpdateLineItemsECPM(updates map[int]float64) error

	// Spend tracking operations
	UpdateLineItemSpend(publisherID, lineItemID int, spend float64) error
	// UpdateLineItemsSpend updates multiple line items' spend values in a single snapshot swap.
	UpdateLineItemsSpend(updates map[int]float64) error

	// CRUD operations for real-time updates
	InsertPublisher(publisher *Publisher) error
	UpdatePublisher(publisher Publisher) error
	DeletePublisher(publisherID int) error

	InsertCampaign(campaign *Campaign) error
	UpdateCampaign(campaign Campaign) error
	DeleteCampaign(campaignID int) error

	InsertLineItem(lineItem *LineItem) error
	UpdateLineItem(lineItem LineItem) error
	DeleteLineItem(lineItemID int) error

	InsertPlacement(placement Placement) error
	UpdatePlacement(placement Placement) error
	DeletePlacement(placementID string) error
}

// dataSnapshot represents an immutable snapshot of all ad data
type dataSnapshot struct {
	lineItems      map[int][]LineItem        // Publisher ID -> LineItems
	lineItemIndex  map[int]map[int]*LineItem // Publisher ID -> LineItem ID -> LineItem
	campaigns      []Campaign
	campaignIndex  map[int]*Campaign // Campaign ID -> Campaign
	publishers     []Publisher
	publisherIndex map[int]*Publisher // Publisher ID -> Publisher
	placements     []Placement
	placementIndex map[string]*Placement
}

// InMemoryAdDataStore implements AdDataStore with atomic snapshot updates
type InMemoryAdDataStore struct {
	// Atomic pointer to current data snapshot
	data atomic.Pointer[dataSnapshot]
}

// NewInMemoryAdDataStore creates a new AdDataStore instance
func NewInMemoryAdDataStore() *InMemoryAdDataStore {
	store := &InMemoryAdDataStore{}
	// Initialize with empty data
	store.data.Store(&dataSnapshot{
		lineItems:      make(map[int][]LineItem),
		lineItemIndex:  make(map[int]map[int]*LineItem),
		campaigns:      make([]Campaign, 0),
		campaignIndex:  make(map[int]*Campaign),
		publishers:     make([]Publisher, 0),
		publisherIndex: make(map[int]*Publisher),
		placements:     make([]Placement, 0),
		placementIndex: make(map[string]*Placement),
	})
	return store
}

// GetLineItem retrieves a specific LineItem by publisher and ID
func (s *InMemoryAdDataStore) GetLineItem(publisherID, lineItemID int) *LineItem {
	data := s.data.Load()
	if byPub, ok := data.lineItemIndex[publisherID]; ok {
		if li, ok := byPub[lineItemID]; ok {
			return li
		}
	}
	return nil
}

// GetLineItemsByPublisher returns all line items for a publisher
func (s *InMemoryAdDataStore) GetLineItemsByPublisher(publisherID int) []LineItem {
	data := s.data.Load()
	if items, ok := data.lineItems[publisherID]; ok {
		// Return a copy to prevent external modification
		result := make([]LineItem, len(items))
		copy(result, items)
		return result
	}
	return nil
}

// GetCampaign retrieves a campaign by ID
func (s *InMemoryAdDataStore) GetCampaign(campaignID int) *Campaign {
	data := s.data.Load()
	if campaign, ok := data.campaignIndex[campaignID]; ok {
		return campaign
	}
	return nil
}

// GetPublisher retrieves a publisher by ID
func (s *InMemoryAdDataStore) GetPublisher(publisherID int) *Publisher {
	data := s.data.Load()
	if publisher, ok := data.publisherIndex[publisherID]; ok {
		return publisher
	}
	return nil
}

// GetPlacement retrieves a placement by ID
func (s *InMemoryAdDataStore) GetPlacement(placementID string) *Placement {
	data := s.data.Load()
	if placement, ok := data.placementIndex[placementID]; ok {
		return placement
	}
	return nil
}

// GetLineItemByID searches for a line item across all publishers (backward compatibility)
func (s *InMemoryAdDataStore) GetLineItemByID(lineItemID int) *LineItem {
	data := s.data.Load()
	for _, byPub := range data.lineItemIndex {
		if li, ok := byPub[lineItemID]; ok {
			return li
		}
	}
	return nil
}

// GetAllPublishers returns all publishers
func (s *InMemoryAdDataStore) GetAllPublishers() []Publisher {
	data := s.data.Load()
	// Return a copy to prevent external modification
	result := make([]Publisher, len(data.publishers))
	copy(result, data.publishers)
	return result
}

// GetAllPublisherIDs returns all publisher IDs that have line items
func (s *InMemoryAdDataStore) GetAllPublisherIDs() []int {
	data := s.data.Load()
	ids := make([]int, 0, len(data.lineItems))
	for pubID := range data.lineItems {
		ids = append(ids, pubID)
	}
	return ids
}

// SetLineItems replaces all line items and rebuilds indexes
func (s *InMemoryAdDataStore) SetLineItems(items []LineItem) error {
	currentData := s.data.Load()
	newData := &dataSnapshot{
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	// Group line items by publisher
	groups := make(map[int][]LineItem)
	for _, li := range items {
		groups[li.PublisherID] = append(groups[li.PublisherID], li)
	}

	newData.lineItems = groups
	newData.lineItemIndex = s.buildLineItemIndex(groups)

	s.data.Store(newData)
	return nil
}

// SetLineItemsForPublisher replaces line items for a specific publisher
func (s *InMemoryAdDataStore) SetLineItemsForPublisher(publisherID int, items []LineItem) error {
	currentData := s.data.Load()

	// Create new line items map
	newLineItems := make(map[int][]LineItem)
	for pubID, lineItems := range currentData.lineItems {
		if pubID == publisherID {
			newLineItems[pubID] = items
		} else {
			newLineItems[pubID] = lineItems
		}
	}

	// If this is a new publisher, add them
	if _, exists := currentData.lineItems[publisherID]; !exists {
		newLineItems[publisherID] = items
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// SetCampaigns replaces all campaigns and rebuilds index
func (s *InMemoryAdDataStore) SetCampaigns(campaigns []Campaign) error {
	currentData := s.data.Load()

	// Build campaign index
	campaignIndex := make(map[int]*Campaign, len(campaigns))
	for i := range campaigns {
		campaignIndex[campaigns[i].ID] = &campaigns[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      campaigns,
		campaignIndex:  campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// SetPublishers replaces all publishers and rebuilds index
func (s *InMemoryAdDataStore) SetPublishers(publishers []Publisher) error {
	currentData := s.data.Load()

	// Build publisher index
	publisherIndex := make(map[int]*Publisher, len(publishers))
	for i := range publishers {
		publisherIndex[publishers[i].ID] = &publishers[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     publishers,
		publisherIndex: publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// SetPlacements replaces all placements and rebuilds index
func (s *InMemoryAdDataStore) SetPlacements(placements []Placement) error {
	currentData := s.data.Load()

	// Build placement index
	placementIndex := make(map[string]*Placement, len(placements))
	for i := range placements {
		placementIndex[placements[i].ID] = &placements[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     placements,
		placementIndex: placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// ReloadAll atomically replaces all data with new values
func (s *InMemoryAdDataStore) ReloadAll(lineItems []LineItem, campaigns []Campaign, publishers []Publisher, placements []Placement) error {
	// Group line items by publisher
	groups := make(map[int][]LineItem)
	for _, li := range lineItems {
		groups[li.PublisherID] = append(groups[li.PublisherID], li)
	}

	// Build campaign index
	campaignIndex := make(map[int]*Campaign, len(campaigns))
	for i := range campaigns {
		campaignIndex[campaigns[i].ID] = &campaigns[i]
	}

	// Build publisher index
	publisherIndex := make(map[int]*Publisher, len(publishers))
	for i := range publishers {
		publisherIndex[publishers[i].ID] = &publishers[i]
	}

	// Build placement index
	placementIndex := make(map[string]*Placement, len(placements))
	for i := range placements {
		placementIndex[placements[i].ID] = &placements[i]
	}

	newData := &dataSnapshot{
		lineItems:      groups,
		lineItemIndex:  s.buildLineItemIndex(groups),
		campaigns:      campaigns,
		campaignIndex:  campaignIndex,
		publishers:     publishers,
		publisherIndex: publisherIndex,
		placements:     placements,
		placementIndex: placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdateLineItemECPM updates the eCPM for a specific line item
func (s *InMemoryAdDataStore) UpdateLineItemECPM(publisherID, lineItemID int, ecpm float64) error {
	currentData := s.data.Load()

	// Create deep copy of line items
	newLineItems := make(map[int][]LineItem)
	for pubID, items := range currentData.lineItems {
		newItems := make([]LineItem, len(items))
		copy(newItems, items)

		// Update eCPM if this is the target publisher
		if pubID == publisherID {
			for i := range newItems {
				if newItems[i].ID == lineItemID {
					newItems[i].ECPM = ecpm
					break
				}
			}
		}

		newLineItems[pubID] = newItems
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdateLineItemsECPM updates multiple line items' eCPM values at once.
func (s *InMemoryAdDataStore) UpdateLineItemsECPM(updates map[int]float64) error {
	if len(updates) == 0 {
		return nil
	}

	currentData := s.data.Load()

	// Create new map, reusing slices when possible
	newLineItems := make(map[int][]LineItem, len(currentData.lineItems))
	for pubID, items := range currentData.lineItems {
		// Determine if any item in this slice requires an update
		needCopy := false
		for i := range items {
			if _, ok := updates[items[i].ID]; ok {
				needCopy = true
				break
			}
		}

		if !needCopy {
			// Reuse existing slice if no updates
			newLineItems[pubID] = items
			continue
		}

		newItems := make([]LineItem, len(items))
		copy(newItems, items)
		for i := range newItems {
			if ecpm, ok := updates[newItems[i].ID]; ok {
				newItems[i].ECPM = ecpm
			}
		}
		newLineItems[pubID] = newItems
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdateLineItemSpend updates the spend for a specific line item
func (s *InMemoryAdDataStore) UpdateLineItemSpend(publisherID, lineItemID int, spend float64) error {
	currentData := s.data.Load()

	// Create deep copy of line items
	newLineItems := make(map[int][]LineItem)
	for pubID, items := range currentData.lineItems {
		newItems := make([]LineItem, len(items))
		copy(newItems, items)

		// Update spend if this is the target publisher
		if pubID == publisherID {
			for i := range newItems {
				if newItems[i].ID == lineItemID {
					newItems[i].Spend = spend
					break
				}
			}
		}

		newLineItems[pubID] = newItems
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdateLineItemsSpend updates multiple line items' spend values at once.
func (s *InMemoryAdDataStore) UpdateLineItemsSpend(updates map[int]float64) error {
	if len(updates) == 0 {
		return nil
	}

	currentData := s.data.Load()

	// Create new map, reusing slices when possible
	newLineItems := make(map[int][]LineItem, len(currentData.lineItems))
	for pubID, items := range currentData.lineItems {
		// Determine if any item in this slice requires an update
		needCopy := false
		for i := range items {
			if _, ok := updates[items[i].ID]; ok {
				needCopy = true
				break
			}
		}

		if !needCopy {
			// Reuse existing slice if no updates
			newLineItems[pubID] = items
			continue
		}

		newItems := make([]LineItem, len(items))
		copy(newItems, items)
		for i := range newItems {
			if spend, ok := updates[newItems[i].ID]; ok {
				newItems[i].Spend = spend
			}
		}
		newLineItems[pubID] = newItems
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// === CRUD Operations ===

// InsertPublisher adds a new publisher to the data store
func (s *InMemoryAdDataStore) InsertPublisher(publisher *Publisher) error {
	currentData := s.data.Load()

	// Create new publisher slice with the additional publisher
	newPublishers := make([]Publisher, len(currentData.publishers)+1)
	copy(newPublishers, currentData.publishers)
	newPublishers[len(currentData.publishers)] = *publisher

	// Rebuild publisher index
	newPublisherIndex := make(map[int]*Publisher, len(newPublishers))
	for i := range newPublishers {
		newPublisherIndex[newPublishers[i].ID] = &newPublishers[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     newPublishers,
		publisherIndex: newPublisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdatePublisher updates an existing publisher in the data store
func (s *InMemoryAdDataStore) UpdatePublisher(publisher Publisher) error {
	currentData := s.data.Load()

	// Create new publisher slice with updated publisher
	newPublishers := make([]Publisher, len(currentData.publishers))
	copy(newPublishers, currentData.publishers)

	// Find and update the publisher
	found := false
	for i := range newPublishers {
		if newPublishers[i].ID == publisher.ID {
			newPublishers[i] = publisher
			found = true
			break
		}
	}

	if !found {
		return ErrNotFound
	}

	// Rebuild publisher index
	newPublisherIndex := make(map[int]*Publisher, len(newPublishers))
	for i := range newPublishers {
		newPublisherIndex[newPublishers[i].ID] = &newPublishers[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     newPublishers,
		publisherIndex: newPublisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// DeletePublisher removes a publisher from the data store
func (s *InMemoryAdDataStore) DeletePublisher(publisherID int) error {
	currentData := s.data.Load()

	// Filter out the deleted publisher
	newPublishers := make([]Publisher, 0, len(currentData.publishers))
	found := false
	for _, pub := range currentData.publishers {
		if pub.ID != publisherID {
			newPublishers = append(newPublishers, pub)
		} else {
			found = true
		}
	}

	if !found {
		return ErrNotFound
	}

	// Rebuild publisher index
	newPublisherIndex := make(map[int]*Publisher, len(newPublishers))
	for i := range newPublishers {
		newPublisherIndex[newPublishers[i].ID] = &newPublishers[i]
	}

	// Also remove line items and campaigns for this publisher
	newLineItems := make(map[int][]LineItem)
	for pubID, items := range currentData.lineItems {
		if pubID != publisherID {
			newLineItems[pubID] = items
		}
	}

	newCampaigns := make([]Campaign, 0, len(currentData.campaigns))
	for _, camp := range currentData.campaigns {
		if camp.PublisherID != publisherID {
			newCampaigns = append(newCampaigns, camp)
		}
	}

	// Rebuild campaigns index
	newCampaignIndex := make(map[int]*Campaign, len(newCampaigns))
	for i := range newCampaigns {
		newCampaignIndex[newCampaigns[i].ID] = &newCampaigns[i]
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      newCampaigns,
		campaignIndex:  newCampaignIndex,
		publishers:     newPublishers,
		publisherIndex: newPublisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// InsertCampaign adds a new campaign to the data store
func (s *InMemoryAdDataStore) InsertCampaign(campaign *Campaign) error {
	currentData := s.data.Load()

	// Create new campaign slice with the additional campaign
	newCampaigns := make([]Campaign, len(currentData.campaigns)+1)
	copy(newCampaigns, currentData.campaigns)
	newCampaigns[len(currentData.campaigns)] = *campaign

	// Rebuild campaign index
	newCampaignIndex := make(map[int]*Campaign, len(newCampaigns))
	for i := range newCampaigns {
		newCampaignIndex[newCampaigns[i].ID] = &newCampaigns[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      newCampaigns,
		campaignIndex:  newCampaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdateCampaign updates an existing campaign in the data store
func (s *InMemoryAdDataStore) UpdateCampaign(campaign Campaign) error {
	currentData := s.data.Load()

	// Create new campaign slice with updated campaign
	newCampaigns := make([]Campaign, len(currentData.campaigns))
	copy(newCampaigns, currentData.campaigns)

	// Find and update the campaign
	found := false
	for i := range newCampaigns {
		if newCampaigns[i].ID == campaign.ID {
			newCampaigns[i] = campaign
			found = true
			break
		}
	}

	if !found {
		return ErrNotFound
	}

	// Rebuild campaign index
	newCampaignIndex := make(map[int]*Campaign, len(newCampaigns))
	for i := range newCampaigns {
		newCampaignIndex[newCampaigns[i].ID] = &newCampaigns[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      newCampaigns,
		campaignIndex:  newCampaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// DeleteCampaign removes a campaign from the data store
func (s *InMemoryAdDataStore) DeleteCampaign(campaignID int) error {
	currentData := s.data.Load()

	// Filter out the deleted campaign
	newCampaigns := make([]Campaign, 0, len(currentData.campaigns))
	found := false
	for _, camp := range currentData.campaigns {
		if camp.ID != campaignID {
			newCampaigns = append(newCampaigns, camp)
		} else {
			found = true
		}
	}

	if !found {
		return ErrNotFound
	}

	// Rebuild campaign index
	newCampaignIndex := make(map[int]*Campaign, len(newCampaigns))
	for i := range newCampaigns {
		newCampaignIndex[newCampaigns[i].ID] = &newCampaigns[i]
	}

	// Also remove line items for this campaign
	newLineItems := make(map[int][]LineItem)
	for pubID, items := range currentData.lineItems {
		filteredItems := make([]LineItem, 0, len(items))
		for _, item := range items {
			if item.CampaignID != campaignID {
				filteredItems = append(filteredItems, item)
			}
		}
		if len(filteredItems) > 0 {
			newLineItems[pubID] = filteredItems
		}
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      newCampaigns,
		campaignIndex:  newCampaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// InsertLineItem adds a new line item to the data store
func (s *InMemoryAdDataStore) InsertLineItem(lineItem *LineItem) error {
	currentData := s.data.Load()

	// Create deep copy of line items
	newLineItems := make(map[int][]LineItem)
	for pubID, items := range currentData.lineItems {
		if pubID == lineItem.PublisherID {
			// Add new line item to this publisher's slice
			newItems := make([]LineItem, len(items)+1)
			copy(newItems, items)
			newItems[len(items)] = *lineItem
			newLineItems[pubID] = newItems
		} else {
			// Copy existing slice
			newLineItems[pubID] = items
		}
	}

	// If publisher doesn't exist in line items, create new slice
	if _, exists := newLineItems[lineItem.PublisherID]; !exists {
		newLineItems[lineItem.PublisherID] = []LineItem{*lineItem}
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdateLineItem updates an existing line item in the data store
func (s *InMemoryAdDataStore) UpdateLineItem(lineItem LineItem) error {
	currentData := s.data.Load()

	// Create deep copy of line items
	newLineItems := make(map[int][]LineItem)
	found := false

	for pubID, items := range currentData.lineItems {
		newItems := make([]LineItem, len(items))
		copy(newItems, items)

		// Update line item if found
		for i := range newItems {
			if newItems[i].ID == lineItem.ID {
				newItems[i] = lineItem
				found = true
				break
			}
		}

		newLineItems[pubID] = newItems
	}

	if !found {
		return ErrNotFound
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// DeleteLineItem removes a line item from the data store
func (s *InMemoryAdDataStore) DeleteLineItem(lineItemID int) error {
	currentData := s.data.Load()

	// Create deep copy of line items, filtering out the deleted item
	newLineItems := make(map[int][]LineItem)
	found := false

	for pubID, items := range currentData.lineItems {
		filteredItems := make([]LineItem, 0, len(items))
		for _, item := range items {
			if item.ID != lineItemID {
				filteredItems = append(filteredItems, item)
			} else {
				found = true
			}
		}
		if len(filteredItems) > 0 {
			newLineItems[pubID] = filteredItems
		}
	}

	if !found {
		return ErrNotFound
	}

	newData := &dataSnapshot{
		lineItems:      newLineItems,
		lineItemIndex:  s.buildLineItemIndex(newLineItems),
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     currentData.placements,
		placementIndex: currentData.placementIndex,
	}

	s.data.Store(newData)
	return nil
}

// InsertPlacement adds a new placement to the data store
func (s *InMemoryAdDataStore) InsertPlacement(placement Placement) error {
	currentData := s.data.Load()

	// Create new placement slice with the additional placement
	newPlacements := make([]Placement, len(currentData.placements)+1)
	copy(newPlacements, currentData.placements)
	newPlacements[len(currentData.placements)] = placement

	// Rebuild placement index
	newPlacementIndex := make(map[string]*Placement, len(newPlacements))
	for i := range newPlacements {
		newPlacementIndex[newPlacements[i].ID] = &newPlacements[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     newPlacements,
		placementIndex: newPlacementIndex,
	}

	s.data.Store(newData)
	return nil
}

// UpdatePlacement updates an existing placement in the data store
func (s *InMemoryAdDataStore) UpdatePlacement(placement Placement) error {
	currentData := s.data.Load()

	// Create new placement slice with updated placement
	newPlacements := make([]Placement, len(currentData.placements))
	copy(newPlacements, currentData.placements)

	// Find and update the placement
	found := false
	for i := range newPlacements {
		if newPlacements[i].ID == placement.ID {
			newPlacements[i] = placement
			found = true
			break
		}
	}

	if !found {
		return ErrNotFound
	}

	// Rebuild placement index
	newPlacementIndex := make(map[string]*Placement, len(newPlacements))
	for i := range newPlacements {
		newPlacementIndex[newPlacements[i].ID] = &newPlacements[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     newPlacements,
		placementIndex: newPlacementIndex,
	}

	s.data.Store(newData)
	return nil
}

// DeletePlacement removes a placement from the data store
func (s *InMemoryAdDataStore) DeletePlacement(placementID string) error {
	currentData := s.data.Load()

	// Filter out the deleted placement
	newPlacements := make([]Placement, 0, len(currentData.placements))
	found := false
	for _, placement := range currentData.placements {
		if placement.ID != placementID {
			newPlacements = append(newPlacements, placement)
		} else {
			found = true
		}
	}

	if !found {
		return ErrNotFound
	}

	// Rebuild placement index
	newPlacementIndex := make(map[string]*Placement, len(newPlacements))
	for i := range newPlacements {
		newPlacementIndex[newPlacements[i].ID] = &newPlacements[i]
	}

	newData := &dataSnapshot{
		lineItems:      currentData.lineItems,
		lineItemIndex:  currentData.lineItemIndex,
		campaigns:      currentData.campaigns,
		campaignIndex:  currentData.campaignIndex,
		publishers:     currentData.publishers,
		publisherIndex: currentData.publisherIndex,
		placements:     newPlacements,
		placementIndex: newPlacementIndex,
	}

	s.data.Store(newData)
	return nil
}

// GetAllCampaigns returns all campaigns
func (s *InMemoryAdDataStore) GetAllCampaigns() []Campaign {
	currentData := s.data.Load()
	return currentData.campaigns
}

// GetAllLineItems returns all line items across all publishers
func (s *InMemoryAdDataStore) GetAllLineItems() []LineItem {
	currentData := s.data.Load()
	var allItems []LineItem
	for _, items := range currentData.lineItems {
		allItems = append(allItems, items...)
	}
	return allItems
}

// GetAllPlacements returns all placements
func (s *InMemoryAdDataStore) GetAllPlacements() []Placement {
	currentData := s.data.Load()
	return currentData.placements
}

// buildLineItemIndex creates the fast lookup index for line items
func (s *InMemoryAdDataStore) buildLineItemIndex(lineItems map[int][]LineItem) map[int]map[int]*LineItem {
	index := make(map[int]map[int]*LineItem, len(lineItems))
	for pubID, items := range lineItems {
		pubIndex := make(map[int]*LineItem, len(items))
		for i := range items {
			pubIndex[items[i].ID] = &items[i]
		}
		index[pubID] = pubIndex
	}
	return index
}
