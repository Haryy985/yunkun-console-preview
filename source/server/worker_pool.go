package server

// workerPool owns its slots from one coordinator goroutine. Retiring workers
// remain in the total until they finish, so a rapid shrink/grow cannot oversubscribe.
type workerPool struct {
	slots map[int]*workerSlot
	next  int
}

type workerSlot struct {
	retire   chan struct{}
	done     chan struct{}
	stopping bool
}

func (p *workerPool) resize(want int, start func(int, *workerSlot) bool) {
	if p.slots == nil {
		p.slots = make(map[int]*workerSlot)
	}
	active := 0
	for id, slot := range p.slots {
		select {
		case <-slot.done:
			delete(p.slots, id)
		default:
			if !slot.stopping {
				active++
			}
		}
	}
	for active > want {
		last := 0
		for id, slot := range p.slots {
			if !slot.stopping && id > last {
				last = id
			}
		}
		slot := p.slots[last]
		slot.stopping = true
		close(slot.retire)
		active--
	}
	for len(p.slots) < want {
		p.next++
		slot := &workerSlot{retire: make(chan struct{}), done: make(chan struct{})}
		if !start(p.next, slot) {
			return
		}
		p.slots[p.next] = slot
	}
}
