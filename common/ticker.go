package common

import (
	"time"

	"github.com/pkg/errors"
)

// Ticker converts time units into ticks. End time of a tick is exclusive.
//
// The consensus layer uses a [Ticker] to map wall-clock time onto integer
// tick numbers; each tick is the unit of pillar election. A momentum
// produced for tick `n` belongs to the half-open window `[ToTime(n).start,
// ToTime(n).end)`.
type Ticker interface {
	// ToTime returns [startTime, endTime) for tick.
	ToTime(tick uint64) (time.Time, time.Time)
	// ToTick returns the tick number that contains t.
	ToTick(t time.Time) uint64
	// TickMultiplier reports how many of this ticker's ticks fit in one of
	// `bigger`'s ticks. Both tickers must share a start time and the
	// bigger ticker's interval must be a whole multiple of this one's.
	TickMultiplier(bigger Ticker) (uint64, error)
}

// ticker is the canonical [Ticker] implementation: a fixed start time and a
// constant interval.
type ticker struct {
	startTime time.Time
	interval  time.Duration
}

// ToTime returns the half-open window for tick.
func (t ticker) ToTime(tick uint64) (time.Time, time.Time) {
	sTime := t.startTime.Add(t.interval * time.Duration(tick))
	eTime := t.startTime.Add(t.interval * time.Duration(tick+1))
	return sTime, eTime
}

// ToTick returns the tick number containing the supplied time. The
// computation truncates toward zero relative to startTime.
func (t ticker) ToTick(time time.Time) uint64 {
	subSec := int64(time.Sub(t.startTime).Seconds())
	i := uint64(subSec) / uint64(t.interval.Seconds())
	return i
}

// TickMultiplier reports how many of this ticker's ticks fit into one tick
// of `bigger`. Returns an error when the start times differ, when this
// ticker is coarser than `bigger`, or when the durations are incompatible.
func (t ticker) TickMultiplier(bigger Ticker) (uint64, error) {
	cStart, cEnd := t.ToTime(0)
	bStart, bEnd := bigger.ToTime(0)
	if cStart != bStart {
		return 0, errors.Errorf("ticker error - start time is different - can't convert ticks")
	}

	cDuration := cEnd.UnixNano() - cStart.UnixNano()
	bDuration := bEnd.UnixNano() - bStart.UnixNano()

	if cDuration > bDuration {
		return 0, errors.Errorf("ticker error - callee has bigger thick duration than argument")
	}

	if bDuration%cDuration != 0 {
		return 0, errors.Errorf("ticker error - can't convert - small duration(ns) %v - big duration(ns) %v", cDuration, bDuration)
	}

	return uint64(bDuration / cDuration), nil
}

// NewTicker constructs a [Ticker] anchored at startTime with the given tick
// interval.
func NewTicker(startTime time.Time, interval time.Duration) Ticker {
	return &ticker{startTime: startTime, interval: interval}
}
