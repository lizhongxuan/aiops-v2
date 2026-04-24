package events

type Subscriber struct {
	ID    uint64
	RunID string
	C     <-chan Event
}
