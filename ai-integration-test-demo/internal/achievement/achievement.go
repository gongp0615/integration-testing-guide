package achievement

import (
	"fmt"

	"github.com/example/ai-integration-test-demo/internal/event"
)

type AchievementState string

const (
	AchLocked   AchievementState = "locked"
	AchUnlocked AchievementState = "unlocked"
)

type Achievement struct {
	AchID int              `json:"achId"`
	Name  string           `json:"name"`
	State AchievementState `json:"state"`
}

type AchievementSystem struct {
	playerID  int
	achs      map[int]*Achievement
	bus       *event.Bus
	hasWeapon bool
	hasArmor  bool
}

func New(playerID int, bus *event.Bus) *AchievementSystem {
	as := &AchievementSystem{
		playerID: playerID,
		achs:     make(map[int]*Achievement),
		bus:      bus,
	}
	bus.Subscribe("task.completed", as.onTaskCompleted)
	bus.Subscribe("item.added", as.onItemAdded)
	bus.Subscribe("equip.success", as.onEquipSuccess)
	return as
}

func (as *AchievementSystem) onTaskCompleted(e event.Event) {
	playerID, _ := e.Data["playerID"].(int)
	if playerID != as.playerID {
		return
	}
	taskID, _ := e.Data["taskID"].(int)

	achMapping := map[int]int{
		3001: 4001,
		3002: 4002,
	}
	if achID, ok := achMapping[taskID]; ok {
		if ach, exists := as.achs[achID]; exists && ach.State == AchLocked {
			as.Unlock(achID)
		}
	}
}

func (as *AchievementSystem) onItemAdded(e event.Event) {
	playerID, _ := e.Data["playerID"].(int)
	if playerID != as.playerID {
		return
	}

	collectorAchID := 4003
	if ach, exists := as.achs[collectorAchID]; exists && ach.State == AchLocked {
		totalItems := 0
		for _, a := range as.achs {
			if a.State == AchUnlocked {
				totalItems++
			}
		}
		if totalItems >= 2 {
			as.Unlock(collectorAchID)
		}
	}
}

func (as *AchievementSystem) onEquipSuccess(e event.Event) {
	playerID, _ := e.Data["playerID"].(int)
	if playerID != as.playerID {
		return
	}
	slot, _ := e.Data["slot"].(string)
	if slot == "weapon" {
		as.hasWeapon = true
	}
	if slot == "armor" {
		as.hasArmor = true
	}

	fullyEquippedAchID := 4004
	if ach, exists := as.achs[fullyEquippedAchID]; exists && ach.State == AchLocked {
		if as.hasWeapon && as.hasArmor {
			as.Unlock(fullyEquippedAchID)
		}
	}
}

func (as *AchievementSystem) AddAchievement(achID int, name string) {
	as.achs[achID] = &Achievement{
		AchID: achID,
		Name:  name,
		State: AchLocked,
	}
	as.bus.AppendLog(fmt.Sprintf("[Achievement] add achievement %d: %s", achID, name))
}

func (as *AchievementSystem) Unlock(achID int) {
	ach, ok := as.achs[achID]
	if !ok || ach.State == AchUnlocked {
		return
	}
	ach.State = AchUnlocked
	as.bus.AppendLog(fmt.Sprintf("[Achievement] unlocked: %s (id=%d)", ach.Name, achID))
	as.bus.Publish(event.Event{
		Type: "achievement.unlocked",
		Data: map[string]any{"playerID": as.playerID, "achID": achID},
	})
}

func (as *AchievementSystem) GetAchievement(achID int) *Achievement {
	return as.achs[achID]
}

func (as *AchievementSystem) AllAchievements() []*Achievement {
	out := make([]*Achievement, 0, len(as.achs))
	for _, a := range as.achs {
		out = append(out, a)
	}
	return out
}
