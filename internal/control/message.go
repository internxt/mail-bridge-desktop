package control

// These names describe what a control message is asking the other side to do or reporting back.
// Both parent and bridge use the same names.
const (
	startSessionType  = "start_session"
	readyType         = "ready"
	sessionUpdateType = "session_updated"
	resyncType        = "resync"
	syncStartedType   = "sync_started"
	syncProgressType  = "sync_progress"
	syncFinishedType  = "sync_finished"
	ackType           = "ack"
	errorType         = "error"
)

const (
	malformedFrameCode   = "malformed_frame"
	badSessionUpdateCode = "bad_session_update"
)
