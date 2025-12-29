package providers

import "time"

// ActionResult represents the result of an action execution
type ActionResult struct {
	// Completed indicates if the action is finished
	Completed bool

	// Progress indicates action progress (0-100)
	Progress int32

	// Message provides status information
	Message string

	// Output contains action-specific output
	Output map[string]string

	// RequeueAfter specifies when to requeue (if not completed)
	RequeueAfter time.Duration
}
