package clock

import (
	"time"
)

type mockTicker struct {
	c    chan time.Time
	stop chan bool

	clock    Clock
	interval time.Duration
	start    time.Time
}

var _ Ticker = new(mockTicker)

// note: this probably does not function the same way as the time.Timer
// in the event that the clock skips more than the timer interval. I've
// not yet dug deep into the runtimeTimer to see how that works.
// PRs are appreciated!
func (m *mockTicker) wait(ready chan<- struct{}) {
	for i := time.Duration(1); true; i++ {
		delta := m.start.Add(m.interval * i).Sub(m.clock.Now())
		afterChan := m.clock.After(delta)

		if i == time.Duration(1) {
			ready <- struct{}{}
		}

		select {
		case <-m.stop:
			return
		case <-afterChan:
			select {
			case m.c <- m.clock.Now():
			case <-m.stop:
				return
			}
		}
	}
}

func (m *mockTicker) Chan() <-chan time.Time {
	return m.c
}

func (m *mockTicker) Stop() {
	m.stop <- true
}

// NewMockTicker creates a new Ticker using the provided Clock. You should not use this
// directly outside of unit tests; use Clock.NewTicker().
func NewMockTicker(c Clock, interval time.Duration) Ticker {
	t := &mockTicker{
		c:        make(chan time.Time),
		stop:     make(chan bool),
		interval: interval,
		start:    c.Now(),
		clock:    c,
	}

	ready := make(chan struct{})
	go t.wait(ready)
	<-ready
	return t
}
