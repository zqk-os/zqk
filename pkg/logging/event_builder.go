package logging

import "time"

// EventBuilder builds structured log messages with fluent field helpers.
type EventBuilder struct {
	msg    string
	fields []Field
}

// NewEvent creates a new event builder for a log message.
func NewEvent(msg string) *EventBuilder {
	return &EventBuilder{msg: msg, fields: make([]Field, 0, 8)}
}

// Message returns the event message.
func (b *EventBuilder) Message() string {
	return b.msg
}

// Fields returns accumulated structured fields.
func (b *EventBuilder) Fields() []Field {
	return b.fields
}

func (b *EventBuilder) String(key, value string) *EventBuilder {
	b.fields = append(b.fields, String(key, value))
	return b
}

func (b *EventBuilder) Int(key string, value int) *EventBuilder {
	b.fields = append(b.fields, Int(key, value))
	return b
}

func (b *EventBuilder) Bool(key string, value bool) *EventBuilder {
	b.fields = append(b.fields, Bool(key, value))
	return b
}

func (b *EventBuilder) Duration(key string, value time.Duration) *EventBuilder {
	b.fields = append(b.fields, String(key, value.String()))
	return b
}

func (b *EventBuilder) BatchID(v string) *EventBuilder   { return b.String(logKeyBatchID, v) }
func (b *EventBuilder) JobID(v string) *EventBuilder     { return b.String(logKeyJobID, v) }
func (b *EventBuilder) JobType(v string) *EventBuilder   { return b.String(logKeyJobType, v) }
func (b *EventBuilder) Category(v string) *EventBuilder  { return b.String(logKeyCategory, v) }
func (b *EventBuilder) Processed(v int) *EventBuilder    { return b.Int(logKeyProcessed, v) }
func (b *EventBuilder) Fixed(v int) *EventBuilder        { return b.Int(logKeyFixed, v) }
func (b *EventBuilder) Failed(v int) *EventBuilder       { return b.Int(logKeyFailed, v) }
func (b *EventBuilder) Skipped(v int) *EventBuilder      { return b.Int(logKeySkipped, v) }
func (b *EventBuilder) ChunkFixed(v int) *EventBuilder   { return b.Int(logKeyChunkFixed, v) }
func (b *EventBuilder) ChunkSkipped(v int) *EventBuilder { return b.Int(logKeyChunkSkipped, v) }
func (b *EventBuilder) ChunkFailed(v int) *EventBuilder  { return b.Int(logKeyChunkFailed, v) }
func (b *EventBuilder) File(v string) *EventBuilder      { return b.String(logKeyFile, v) }

// Emit helpers
func (b *EventBuilder) Info(l Logger) {
	l.Info(b.msg, b.fields...)
}

func (b *EventBuilder) Debug(l Logger) {
	l.Debug(b.msg, b.fields...)
}

func (b *EventBuilder) Warn(l Logger) {
	l.Warn(b.msg, b.fields...)
}

func (b *EventBuilder) Error(l Logger, err error) {
	l.Error(b.msg, err, b.fields...)
}
