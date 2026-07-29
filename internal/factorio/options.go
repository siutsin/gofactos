// This file defines caller-controlled options for blueprint generation.
package factorio

// Option configures blueprint generation. Options thread from the CLI through
// Compile into the selector before the body is lowered.
type Option func(*selector)

// WithParams lets callers seed parameter sources by name. Values are baked into
// the parameter
// constant combinators (--set). An omitted parameter keeps the default seed of
// 1. A name that matches no parameter is rejected by Select.
func WithParams(values map[string]int) Option {
	return func(s *selector) { s.paramValues = values }
}

// WithFastClock lets callers favour execution speed over a one-hertz display.
// It selects the fastest period supported by the runtime circuits and has no
// effect when the generated blueprint needs no clock.
func WithFastClock() Option {
	return func(s *selector) { s.clockPeriod = fastClockPeriod }
}
