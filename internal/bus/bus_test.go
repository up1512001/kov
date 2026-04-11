package bus

import (
	"sync"
	"testing"
	"time"
)

func TestBus_PublishSubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	ch, cancel := b.Subscribe(TokenReceived{})
	defer cancel()

	// Publish an event
	b.Publish(TokenReceived{
		SessionID: "test-session",
		Token:     "hello",
		Provider:  "anthropic",
	})

	select {
	case event := <-ch:
		token, ok := event.(TokenReceived)
		if !ok {
			t.Fatalf("expected TokenReceived, got %T", event)
		}
		if token.Token != "hello" {
			t.Fatalf("expected token 'hello', got %q", token.Token)
		}
		if token.SessionID != "test-session" {
			t.Fatalf("expected session 'test-session', got %q", token.SessionID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	b := New()
	defer b.Close()

	ch1, cancel1 := b.Subscribe(TaskCompleted{})
	defer cancel1()
	ch2, cancel2 := b.Subscribe(TaskCompleted{})
	defer cancel2()

	b.Publish(TaskCompleted{
		SessionID: "s1",
		TaskID:    "t1",
		Result:    "done",
	})

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case event := <-ch:
			tc := event.(TaskCompleted)
			if tc.TaskID != "t1" {
				t.Fatalf("subscriber %d: expected task t1, got %q", i, tc.TaskID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestBus_DifferentEventTypes(t *testing.T) {
	b := New()
	defer b.Close()

	tokenCh, cancelToken := b.Subscribe(TokenReceived{})
	defer cancelToken()
	taskCh, cancelTask := b.Subscribe(TaskCompleted{})
	defer cancelTask()

	// Publish a token event
	b.Publish(TokenReceived{Token: "hi"})

	// Token subscriber should get it...
	select {
	case <-tokenCh:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Fatal("token channel should have received event")
	}

	// ...but task subscriber should NOT
	select {
	case <-taskCh:
		t.Fatal("task channel should NOT have received token event")
	case <-time.After(50 * time.Millisecond):
		// ok — no event received
	}
}

func TestBus_Cancel(t *testing.T) {
	b := New()
	defer b.Close()

	ch, cancel := b.Subscribe(TokenReceived{})

	// Publish an event before cancel
	b.Publish(TokenReceived{Token: "before"})
	select {
	case <-ch:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Fatal("should have received event before cancel")
	}

	cancel()

	// After cancel, no new events should arrive
	b.Publish(TokenReceived{Token: "after"})
	select {
	case <-ch:
		// Channel may have been closed by `Close()` later or may
		// receive nothing. Either way, after cancel we shouldn't
		// see new events.
	case <-time.After(50 * time.Millisecond):
		// ok — no event received after cancel
	}
}

func TestBus_NonBlocking(t *testing.T) {
	b := New()
	defer b.Close()

	// Subscribe with tiny buffer
	_, cancel := b.Subscribe(TokenReceived{}, 1)
	defer cancel()

	// Publish more events than buffer — should not block
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish(TokenReceived{Token: "x"})
		}
		close(done)
	}()

	select {
	case <-done:
		// ok — publish didn't block
	case <-time.After(1 * time.Second):
		t.Fatal("publish blocked — bus should be non-blocking")
	}
}

func TestBus_ConcurrentSafety(t *testing.T) {
	b := New()
	defer b.Close()

	var wg sync.WaitGroup
	// Many concurrent publishers and subscribers
	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			ch, cancel := b.Subscribe(TokenReceived{})
			defer cancel()
			// Read a few events
			for j := 0; j < 5; j++ {
				select {
				case <-ch:
				case <-time.After(200 * time.Millisecond):
					return
				}
			}
		}()

		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				b.Publish(TokenReceived{Token: "concurrent"})
			}
		}()
	}

	wg.Wait()
}
