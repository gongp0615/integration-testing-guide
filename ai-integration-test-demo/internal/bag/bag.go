package bag

import (
	"fmt"

	"github.com/example/ai-integration-test-demo/internal/event"
)

type Item struct {
	ItemID int `json:"itemId"`
	Count  int `json:"count"`
}

type Bag struct {
	playerID int
	items    map[int]*Item
	bus      *event.Bus
}

func New(playerID int, bus *event.Bus) *Bag {
	return &Bag{
		playerID: playerID,
		items:    make(map[int]*Item),
		bus:      bus,
	}
}

func (b *Bag) AddItem(itemID, count int) {
	if count <= 0 {
		b.bus.AppendLog(fmt.Sprintf("[Bag] reject add item %d: invalid count %d", itemID, count))
		return
	}
	if it, ok := b.items[itemID]; ok {
		it.Count += count
	} else {
		b.items[itemID] = &Item{ItemID: itemID, Count: count}
	}
	b.bus.AppendLog(fmt.Sprintf("[Bag] add item %d x%d", itemID, count))
	b.bus.Publish(event.Event{
		Type: "item.added",
		Data: map[string]any{"playerID": b.playerID, "itemID": itemID, "count": count},
	})
}

func (b *Bag) RemoveItem(itemID, count int) bool {
	it, ok := b.items[itemID]
	if !ok || it.Count < count {
		b.bus.AppendLog(fmt.Sprintf("[Bag] remove item %d x%d failed: not enough", itemID, count))
		return false
	}
	it.Count -= count
	b.bus.AppendLog(fmt.Sprintf("[Bag] remove item %d x%d", itemID, count))
	if it.Count == 0 {
		delete(b.items, itemID)
	}
	b.bus.Publish(event.Event{
		Type: "item.removed",
		Data: map[string]any{"playerID": b.playerID, "itemID": itemID, "count": count},
	})
	return true
}

func (b *Bag) GetItem(itemID int) *Item {
	return b.items[itemID]
}

func (b *Bag) AllItems() []*Item {
	out := make([]*Item, 0, len(b.items))
	for _, it := range b.items {
		out = append(out, it)
	}
	return out
}
