package bus

import (
	"sync"

	"github.com/jpackagejasonc/aitop/internal/provider"
)

// Bus is a simple pub/sub event bus backed by channels.
type Bus struct {
	mu          sync.Mutex
	subscribers []chan provider.Event
}

// New returns an initialised Bus.
func New() *Bus {
	return &Bus{}
}

// Subscribe creates a buffered subscriber channel and registers it with the bus.
// The caller must eventually call Unsubscribe to release resources.
func (b *Bus) Subscribe() <-chan provider.Event {
	ch := make(chan provider.Event, 100)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// Publish sends the event to every subscriber. Slow subscribers are skipped
// (non-blocking send) so that a stalled consumer cannot block the publisher.
func (b *Bus) Publish(e provider.Event) {
	b.mu.Lock()
	subs := make([]chan provider.Event, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// subscriber buffer full – drop the event rather than blocking
		}
	}
}

// Unsubscribe removes the channel from the subscriber list and closes it.
func (b *Bus) Unsubscribe(ch <-chan provider.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, s := range b.subscribers {
		if s == ch {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			close(s)
			return
		}
	}
}
