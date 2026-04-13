package event

import "sync"

type Event struct {
	Type string
	Data map[string]any
}

type Handler func(Event)

type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	log      []string
	logMu    sync.Mutex
}

func NewBus() *Bus {
	return &Bus{
		handlers: make(map[string][]Handler),
	}
}

func (b *Bus) Subscribe(eventType string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], h)
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	handlers := make([]Handler, 0)
	if hs, ok := b.handlers[e.Type]; ok {
		handlers = append(handlers, hs...)
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		h(e)
	}
}

func (b *Bus) AppendLog(msg string) {
	b.logMu.Lock()
	defer b.logMu.Unlock()
	b.log = append(b.log, msg)
}

func (b *Bus) DrainLog() []string {
	b.logMu.Lock()
	defer b.logMu.Unlock()
	out := make([]string, len(b.log))
	copy(out, b.log)
	b.log = b.log[:0]
	return out
}
