package breakpoint

import (
	"github.com/example/ai-integration-test-demo/internal/event"
)

type PendingOp struct {
	Execute func()
}

type Controller struct {
	bus     *event.Bus
	queue   chan PendingOp
	running bool
}

func NewController(bus *event.Bus) *Controller {
	return &Controller{
		bus:   bus,
		queue: make(chan PendingOp, 256),
	}
}

func (c *Controller) Enqueue(op PendingOp) {
	c.queue <- op
}

func (c *Controller) Next() []string {
	select {
	case op := <-c.queue:
		op.Execute()
	default:
	}
	return c.bus.DrainLog()
}

func (c *Controller) PendingCount() int {
	return len(c.queue)
}
