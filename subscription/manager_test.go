package subscription

import (
	"encoding/json"
	"sync"
	"testing"
)

// makeMsg creates a simple test event.
func makeMsg(n int) json.RawMessage {
	data, _ := json.Marshal(map[string]any{"n": n})
	return data
}

func TestSubscribeBroadcast(t *testing.T) {
	m := NewManager()
	ch := make(chan json.RawMessage, 16)

	c := m.Subscribe(ch, nil)
	if c == nil {
		t.Fatal("Subscribe returned nil")
	}
	if m.Count() != 1 {
		t.Fatalf("Count = %d, want 1", m.Count())
	}

	m.Broadcast("raw", makeMsg(1))

	select {
	case msg := <-ch:
		var v map[string]any
		if err := json.Unmarshal(msg, &v); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if v["n"].(float64) != 1 {
			t.Fatalf("got %v, want 1", v["n"])
		}
	default:
		t.Fatal("did not receive broadcast")
	}
}

func TestUnsubscribeStopsEvents(t *testing.T) {
	m := NewManager()
	ch := make(chan json.RawMessage, 16)

	c := m.Subscribe(ch, nil)
	m.Broadcast("raw", makeMsg(1))

	m.Unsubscribe(c)
	if m.Count() != 0 {
		t.Fatalf("Count = %d, want 0 after unsubscribe", m.Count())
	}

	m.Broadcast("raw", makeMsg(2))

	// Drain the first message
	<-ch

	select {
	case msg := <-ch:
		t.Fatalf("received event after unsubscribe: %s", msg)
	default:
		// good — no more events
	}
}

func TestMultiClientBroadcast(t *testing.T) {
	m := NewManager()
	ch1 := make(chan json.RawMessage, 16)
	ch2 := make(chan json.RawMessage, 16)
	ch3 := make(chan json.RawMessage, 16)

	m.Subscribe(ch1, nil)
	m.Subscribe(ch2, nil)
	m.Subscribe(ch3, nil)

	if m.Count() != 3 {
		t.Fatalf("Count = %d, want 3", m.Count())
	}

	m.Broadcast("raw", makeMsg(42))

	for i, ch := range []chan json.RawMessage{ch1, ch2, ch3} {
		select {
		case msg := <-ch:
			var v map[string]any
			json.Unmarshal(msg, &v)
			if v["n"].(float64) != 42 {
				t.Fatalf("client %d: got %v, want 42", i, v["n"])
			}
		default:
			t.Fatalf("client %d: did not receive broadcast", i)
		}
	}
}

func TestSlowClientDropPolicy(t *testing.T) {
	m := NewManager()
	// Buffer size 2 — only 2 messages fit before the reader must drain.
	ch := make(chan json.RawMessage, 2)

	c := m.Subscribe(ch, nil)

	// Broadcast 5 messages without draining. Buffer holds 2, so 3 are dropped.
	for i := 0; i < 5; i++ {
		m.Broadcast("raw", makeMsg(i))
	}

	// Should have received exactly 2 (buffer size).
	received := 0
	drain:
	for {
		select {
		case <-ch:
			received++
		default:
			break drain
		}
	}
	if received != 2 {
		t.Fatalf("received %d, want 2 (buffer size)", received)
	}
	if c.Dropped() != 3 {
		t.Fatalf("dropped = %d, want 3", c.Dropped())
	}
}

func TestTypeFiltering(t *testing.T) {
	m := NewManager()
	chRaw := make(chan json.RawMessage, 16)
	chOther := make(chan json.RawMessage, 16)

	m.Subscribe(chRaw, []string{"raw"})
	m.Subscribe(chOther, []string{"other"})

	// Broadcast "raw" — only chRaw should get it.
	m.Broadcast("raw", makeMsg(1))
	select {
	case <-chRaw:
	default:
		t.Fatal("raw subscriber did not get raw event")
	}
	select {
	case msg := <-chOther:
		t.Fatalf("other subscriber got raw event: %s", msg)
	default:
		// good
	}

	// Broadcast "other" — only chOther should get it.
	m.Broadcast("other", makeMsg(2))
	select {
	case msg := <-chRaw:
		t.Fatalf("raw subscriber got other event: %s", msg)
	default:
		// good
	}
	select {
	case <-chOther:
	default:
		t.Fatal("other subscriber did not get other event")
	}
}

func TestConcurrentBroadcast(t *testing.T) {
	m := NewManager()

	// Subscribe 10 clients.
	var clients []chan json.RawMessage
	for i := 0; i < 10; i++ {
		ch := make(chan json.RawMessage, 100)
		m.Subscribe(ch, nil)
		clients = append(clients, ch)
	}

	// Broadcast from multiple goroutines concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Broadcast("raw", makeMsg(n))
		}(i)
	}
	wg.Wait()

	// Each client should have received all 20 messages.
	for i, ch := range clients {
		count := 0
	drain:
		for {
			select {
			case <-ch:
				count++
			default:
				break drain
			}
		}
		if count != 20 {
			t.Fatalf("client %d: received %d, want 20", i, count)
		}
	}
}

func TestUnsubscribeNilSafe(t *testing.T) {
	m := NewManager()
	m.Unsubscribe(nil) // should not panic
}
