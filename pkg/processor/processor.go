package processor

import (
	"context"
	"encoding/json"
	"github.com/raczu/kube2kafka/pkg/circular"
	"github.com/raczu/kube2kafka/pkg/kube"
	"github.com/raczu/kube2kafka/pkg/kube/watcher"
	log "github.com/raczu/kube2kafka/pkg/logger"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"time"
)

const (
	DefaultKafkaMessageBufferCap = 128
)

type KafkaMessageBuffer = circular.RingBuffer[*kafka.Message]

func NewKafkaMessageBuffer(capacity int) *KafkaMessageBuffer {
	return circular.NewRingBuffer[*kafka.Message](capacity)
}

type Option func(*Processor)

type Processor struct {
	source     *watcher.EventBuffer
	output     *KafkaMessageBuffer
	filters    []Filter
	customizer *PayloadCustomizer
	logger     *zap.Logger
}

func New(source *watcher.EventBuffer, opts ...Option) *Processor {
	p := &Processor{
		source: source,
		output: NewKafkaMessageBuffer(DefaultKafkaMessageBufferCap),
		logger: log.New().Named("processor"),
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.customizer != nil {
		sublogger := p.logger.Named("customizer")
		// Wrap the customizer to log errors and fallback to default field selection.
		fallback := func(event *kube.EnhancedEvent, err error) map[string]string {
			sublogger.Warn(
				"error occurred while selecting fields from the event,"+
					" falling back to default fields selection",
				zap.Error(err),
			)
			return RelevantFieldSelection(event)
		}
		p.customizer.OnSelectionError = fallback
	}
	return p
}

// GetBuffer returns the buffer where the processor writes processed events as
// ready to be sent kafka.Message.
func (p *Processor) GetBuffer() *KafkaMessageBuffer {
	return p.output
}

func (p *Processor) writeToBuffer(event *kube.EnhancedEvent) {
	var payload []byte
	if p.customizer != nil {
		customized := p.customizer.Customize(event)
		payload, _ = json.Marshal(customized)
	} else {
		payload, _ = json.Marshal(event)
	}

	message := &kafka.Message{
		Key:   []byte(event.UID),
		Value: payload,
	}
	p.output.Write(message)
}

// Process reads events from the source buffer, applies the filters, customizes the event
// payload and writes it to the output buffer as ready to be sent kafka.Message.
func (p *Processor) Process(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			event, ok := p.source.Read()
			if !ok {
				continue
			}

			if len(p.filters) > 0 && !AnyFilterMatches(p.filters, event) {
				p.logger.Debug("event filtered out",
					zap.String("namespace", event.Namespace),
					zap.String("name", event.Name),
					zap.String("reason", event.Reason),
					zap.String("regarding", event.InvolvedObject.Name),
				)
				continue
			}
			p.writeToBuffer(event)
		}
	}
}

// WithLogger sets the logger to be used by the processor.
func WithLogger(logger *zap.Logger) Option {
	return func(p *Processor) {
		p.logger = logger
	}
}

// WithFilters sets the filters to be applied to the events before processing.
func WithFilters(filters []Filter) Option {
	return func(p *Processor) {
		p.filters = filters
	}
}

// WithSelectors sets the selectors used to customize the final event payload.
func WithSelectors(selectors []Selector) Option {
	return func(p *Processor) {
		p.customizer = &PayloadCustomizer{Selectors: selectors}
	}
}

// WriteTo sets the buffer where the processor writes processed events as
// ready to be sent kafka.Message.
func WriteTo(output *KafkaMessageBuffer) Option {
	return func(p *Processor) {
		p.output = output
	}
}
