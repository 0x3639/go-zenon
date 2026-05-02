package pillar

import "github.com/pkg/errors"

// Admission-rule errors returned by [manager.shouldProcess]. None
// of these indicate a bug — they are expected outcomes for events
// the local node should not act on.
var (
	// ErrSyncNotDone — node is still in catch-up; producing now
	// would risk minting on a stale chain.
	ErrSyncNotDone = errors.Errorf("sync is not done")
	// ErrPillarNotDefined — non-producing node received a
	// ProducerEvent (should be rare; ignored gracefully).
	ErrPillarNotDefined = errors.Errorf("pillar has no producer address defined")
	// ErrNotOurEvent — slot is for a different elected pillar.
	ErrNotOurEvent = errors.Errorf("not our event")
	// ErrEventHasNotStarted — slot StartTime is in the future
	// (clock skew or premature dispatch).
	ErrEventHasNotStarted = errors.Errorf("current time is before start time")
	// ErrEventEnded — slot EndTime has already passed.
	ErrEventEnded = errors.Errorf("current time is after the event's finish time time")
)
