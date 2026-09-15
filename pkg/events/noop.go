package events

import "context"

// NopPublisher is the default Publisher. It discards every event, so with no
// events driver configured the application behaves exactly as it would without
// any event publishing at all. It is the zero-dependency default wired by
// cmd/server when EVENTS_DRIVER is unset or "none".
type NopPublisher struct{}

// Publish discards the event and always succeeds.
func (NopPublisher) Publish(context.Context, Event) error { return nil }

// Close is a no-op.
func (NopPublisher) Close() error { return nil }

// Compile-time assertion that NopPublisher satisfies Publisher.
var _ Publisher = NopPublisher{}
