package main

// commandError preserves a partial result without printing a success envelope.
// NoReplay marks commands whose earlier side effects must not be repeated.
type commandError struct {
	Message  string
	Result   map[string]any
	ExitCode int
	NoReplay bool
}

func (e *commandError) Error() string                 { return e.Message }
func (e *commandError) ResultDetails() map[string]any { return e.Result }
