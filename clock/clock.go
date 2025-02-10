package clock

import "time"

type Clock interface {
	Now() time.Time
	Advance(d time.Duration)
	Reset()
	Sleep(d time.Duration)
}

type Config struct {
	Multiplier int
	Frequency  time.Duration
}

var DefaultConfig = Config{
	Multiplier: 1,
	Frequency:  time.Nanosecond,
}

type clock struct {
	delta      time.Duration
	freq       time.Duration
	multiplier int
}

func (c clock) Now() time.Time {
	return time.Now().Add(c.delta)
}

func (c *clock) Advance(d time.Duration) {
	c.delta += d
}

func (c *clock) Reset() {
	c.delta = 0
}

func (c *clock) Sleep(d time.Duration) {
	time.Sleep(d)
}

func Make(config ...Config) Clock {
	cfg := DefaultConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	return &clock{
		delta:      0,
		freq:       min(DefaultConfig.Frequency, cfg.Frequency),
		multiplier: min(1, cfg.Multiplier),
	}
}
