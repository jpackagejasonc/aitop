package bus

import (
	"testing"
	"time"

	"github.com/jpackagejasonc/aitop/internal/provider"
)

func event(sessionID string) provider.Event {
	return provider.Event{
		Type:      provider.EventToolCall,
		SessionID: sessionID,
	}
}

func TestPublish_EmptyBusDoesNotPanic(t *testing.T) {
	b := New()
	b.Publish(event("s1")) // must not panic
}

func TestSubscribe_ReceivesPublishedEvent(t *testing.T) {
	b := New()
	ch := b.Subscribe()

	b.Publish(event("s1"))

	select {
	case e := <-ch:
		if e.SessionID != "s1" {
			t.Errorf("SessionID: want s1, got %s", e.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribe_MultipleSubscribersEachReceive(t *testing.T) {
	b := New()
	ch1 := b.Subscribe()
	ch2 := b.Subscribe()

	b.Publish(event("s1"))

	for i, ch := range []<-chan provider.Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.SessionID != "s1" {
				t.Errorf("subscriber %d: want s1, got %s", i, e.SessionID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestPublish_FullSubscriberDropsWithoutBlocking(t *testing.T) {
	b := New()
	ch := b.Subscribe() // buffer size 100

	// Fill the buffer completely.
	for i := 0; i < 100; i++ {
		b.Publish(event("fill"))
	}

	// This publish must return immediately rather than blocking.
	done := make(chan struct{})
	go func() {
		b.Publish(event("overflow"))
		close(done)
	}()

	select {
	case <-done:
		// good — returned without blocking
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a full subscriber")
	}

	// The channel should still have exactly 100 events (the overflow was dropped).
	if len(ch) != 100 {
		t.Errorf("channel length: want 100, got %d", len(ch))
	}
}

func TestPublish_SlowSubscriberDoesNotBlockFastOne(t *testing.T) {
	b := New()
	slow := b.Subscribe()
	fast := b.Subscribe()

	// Fill the slow subscriber's buffer.
	for i := 0; i < 100; i++ {
		b.Publish(event("fill"))
	}

	// Drain the fast subscriber so it has room.
	for len(fast) > 0 {
		<-fast
	}

	// Now publish one more — slow is full, fast is empty.
	b.Publish(event("new"))

	// Fast subscriber must receive it.
	select {
	case e := <-fast:
		if e.SessionID != "new" {
			t.Errorf("fast: want new, got %s", e.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("fast subscriber did not receive event")
	}

	// Slow subscriber must not have received the overflow.
	if len(slow) != 100 {
		t.Errorf("slow buffer: want 100, got %d", len(slow))
	}
}

func TestUnsubscribe_ChannelClosedAndRemoved(t *testing.T) {
	b := New()
	ch := b.Subscribe()
	b.Unsubscribe(ch)

	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	default:
		t.Error("expected closed channel to be readable")
	}

	// Subsequent publishes must not panic or attempt to send to the closed channel.
	b.Publish(event("after-unsub"))
}

func TestUnsubscribe_OnlyRemovesTargetSubscriber(t *testing.T) {
	b := New()
	ch1 := b.Subscribe()
	ch2 := b.Subscribe()

	b.Unsubscribe(ch1)
	b.Publish(event("s1"))

	select {
	case e := <-ch2:
		if e.SessionID != "s1" {
			t.Errorf("ch2: want s1, got %s", e.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("ch2 did not receive event after ch1 was unsubscribed")
	}
}
