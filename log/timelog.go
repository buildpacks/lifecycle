package log

import "time"

// Chronit is a made up word. I guess it's short for chronological unit because it measures time or something anyway just call NewFuncTimer.
type Chronit struct {
	StartTime    time.Time
	EndTime      time.Time
	Log          Logger
	FunctionName string
}

// NewFuncTimer initializes a chronological measuring tool, logs out the start time, and returns it to you for later.
func NewFuncTimer(funcName string, lager Logger) Chronit {
	c := Chronit{Log: lager, FunctionName: funcName}
	c.RecordStart()
	return c
}

// RecordStart grabs the current time and logs it, but it will be called for you if you use the NewFuncTimer convenience function.
func (c *Chronit) RecordStart() {
	c.StartTime = time.Now()
	c.Log.Infof("Timer: %s started at %s", c.FunctionName, c.StartTime.Format(time.RFC3339))
}

// RecordEnd is probably the call you want to defer right after making one of these puppies via NewFuncTimer.
// the EndTime will be populated just in case you'll keep the object in scope for later.
func (c *Chronit) RecordEnd() {
	c.EndTime = time.Now()
	c.Log.Infof("Timer: %s ran for %v and ended at %s", c.FunctionName, c.EndTime.Sub(c.StartTime), c.EndTime.Format(time.RFC3339))
}
