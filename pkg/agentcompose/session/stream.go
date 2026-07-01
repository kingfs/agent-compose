package session

import (
	"strings"
	"sync"

	"github.com/samber/do/v2"
)

const sessionStreamBufferSize = 256

type WatchEventType int

const (
	WatchEventTypeUnspecified WatchEventType = iota
	WatchEventTypeSessionUpdated
	WatchEventTypeCellStarted
	WatchEventTypeCellOutput
	WatchEventTypeCellCompleted
	WatchEventTypeEventAdded
)

type WatchEvent struct {
	SessionID string
	EventType WatchEventType
	Session   *SessionSummary
	Cell      *NotebookCell
	Event     *SessionEvent
	CellID    string
	Chunk     string
	IsStderr  bool
}

type SessionStreamBroker struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[string]map[int]chan WatchEvent
}

func NewSessionStreamBroker(do.Injector) (*SessionStreamBroker, error) {
	return &SessionStreamBroker{subscribers: map[string]map[int]chan WatchEvent{}}, nil
}

func (b *SessionStreamBroker) Subscribe(sessionID string) (<-chan WatchEvent, func()) {
	sessionID = strings.TrimSpace(sessionID)
	ch := make(chan WatchEvent, sessionStreamBufferSize)
	if b == nil || sessionID == "" {
		close(ch)
		return ch, func() {}
	}
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	if b.subscribers[sessionID] == nil {
		b.subscribers[sessionID] = map[int]chan WatchEvent{}
	}
	b.subscribers[sessionID][id] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		items := b.subscribers[sessionID]
		if items == nil {
			return
		}
		item, ok := items[id]
		if !ok {
			return
		}
		delete(items, id)
		close(item)
		if len(items) == 0 {
			delete(b.subscribers, sessionID)
		}
	}
}

func (b *SessionStreamBroker) PublishSessionUpdated(summary *SessionSummary) {
	if summary == nil {
		return
	}
	b.publish(WatchEvent{
		SessionID: summary.ID,
		EventType: WatchEventTypeSessionUpdated,
		Session:   CloneSummary(summary),
	})
}

func (b *SessionStreamBroker) PublishCellStarted(sessionID string, cell NotebookCell) {
	b.publish(WatchEvent{
		SessionID: strings.TrimSpace(sessionID),
		EventType: WatchEventTypeCellStarted,
		Cell:      CloneNotebookCell(&cell),
	})
}

func (b *SessionStreamBroker) PublishCellOutput(sessionID, cellID, chunk string, isStderr bool) {
	b.publish(WatchEvent{
		SessionID: strings.TrimSpace(sessionID),
		EventType: WatchEventTypeCellOutput,
		CellID:    strings.TrimSpace(cellID),
		Chunk:     chunk,
		IsStderr:  isStderr,
	})
}

func (b *SessionStreamBroker) PublishCellCompleted(sessionID string, cell NotebookCell) {
	b.publish(WatchEvent{
		SessionID: strings.TrimSpace(sessionID),
		EventType: WatchEventTypeCellCompleted,
		Cell:      CloneNotebookCell(&cell),
	})
}

func (b *SessionStreamBroker) PublishEventAdded(sessionID string, event SessionEvent) {
	b.publish(WatchEvent{
		SessionID: strings.TrimSpace(sessionID),
		EventType: WatchEventTypeEventAdded,
		Event:     CloneEvent(&event),
	})
}

func (b *SessionStreamBroker) publish(event WatchEvent) {
	if b == nil || strings.TrimSpace(event.SessionID) == "" {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers[event.SessionID] {
		select {
		case ch <- event:
		default:
		}
	}
}

func CloneSummary(summary *SessionSummary) *SessionSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	if len(summary.Tags) > 0 {
		cloned.Tags = append([]SessionTag(nil), summary.Tags...)
	}
	return &cloned
}

func CloneNotebookCell(cell *NotebookCell) *NotebookCell {
	if cell == nil {
		return nil
	}
	cloned := *cell
	if cell.AgentResume != nil {
		resume := *cell.AgentResume
		if len(cell.AgentResume.SessionJSONLPaths) > 0 {
			resume.SessionJSONLPaths = append([]string(nil), cell.AgentResume.SessionJSONLPaths...)
		}
		cloned.AgentResume = &resume
	}
	return &cloned
}

func CloneEvent(event *SessionEvent) *SessionEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	return &cloned
}
