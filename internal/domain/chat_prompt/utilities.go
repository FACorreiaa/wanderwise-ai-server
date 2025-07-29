package chat_prompt

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (l *Service) sendEvent(ctx context.Context, ch chan<- StreamEvent, event StreamEvent, retries int) bool {
	for i := 0; i < retries; i++ {
		if event.EventID == "" {
			event.EventID = uuid.New().String()
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now()
		}

		select {
		case <-ctx.Done():
			l.logger.Warn("Context cancelled, not sending stream event", zap.String("eventType", event.Type))
			l.deadLetterCh <- event
			return false
		default:
			select {
			case ch <- event:
				return true
			case <-ctx.Done():
				l.logger.Warn("Context cancelled while trying to send stream event", zap.String("eventType", event.Type))
				l.deadLetterCh <- event
				return false
			case <-time.After(2 * time.Second):
				l.logger.Warn("Dropped stream event due to slow consumer or blocked channel (timeout)", zap.String("eventType", event.Type))
				l.deadLetterCh <- event
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (l *Service) processDeadLetterQueue() {
	for event := range l.deadLetterCh {
		l.logger.Error("Unprocessed event sent to dead letter queue", zap.Any("event", event))
	}
}
