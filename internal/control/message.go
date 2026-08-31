package control

// These names describe what a control message is asking the other side to do or reporting back.
// Both parent and bridge use the same names.
const (
	startSessionType  = "start_session"
	readyType         = "ready"
	sessionUpdateType = "session_updated"
	ackType           = "ack"
	errorType         = "error"
)
