package player

import (
	"github.com/example/ai-integration-test-demo/internal/achievement"
	"github.com/example/ai-integration-test-demo/internal/bag"
	"github.com/example/ai-integration-test-demo/internal/equipment"
	"github.com/example/ai-integration-test-demo/internal/event"
	"github.com/example/ai-integration-test-demo/internal/mail"
	"github.com/example/ai-integration-test-demo/internal/signin"
	"github.com/example/ai-integration-test-demo/internal/task"
)

type Player struct {
	ID           int
	Bag          *bag.Bag
	Tasks        *task.TaskSystem
	Achievements *achievement.AchievementSystem
	Equipment    *equipment.EquipmentSystem
	SignIn       *signin.SignInSystem
	Mail         *mail.MailSystem
}

type Manager struct {
	players map[int]*Player
	bus     *event.Bus
}

func NewManager(bus *event.Bus) *Manager {
	return &Manager{
		players: make(map[int]*Player),
		bus:     bus,
	}
}

func (m *Manager) CreatePlayer(id int) *Player {
	p := &Player{
		ID:           id,
		Bag:          bag.New(id, m.bus),
		Tasks:        task.New(id, m.bus),
		Achievements: achievement.New(id, m.bus),
		Equipment:    equipment.New(id, m.bus),
		SignIn:       signin.New(id, m.bus),
		Mail:         mail.New(id, m.bus),
	}
	m.players[id] = p

	p.Tasks.AddTask(3001, 1)
	p.Tasks.AddTask(3002, 2)

	p.Achievements.AddAchievement(4001, "first_task")
	p.Achievements.AddAchievement(4002, "task_master")
	p.Achievements.AddAchievement(4003, "collector_100")
	p.Achievements.AddAchievement(4004, "fully_equipped")

	return p
}

func (m *Manager) GetPlayer(id int) *Player {
	return m.players[id]
}

func (m *Manager) AllPlayerIDs() []int {
	ids := make([]int, 0, len(m.players))
	for id := range m.players {
		ids = append(ids, id)
	}
	return ids
}
