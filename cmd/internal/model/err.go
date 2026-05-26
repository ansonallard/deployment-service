package model

type unionMemberNotPresent struct{}

func (u *unionMemberNotPresent) Error() string {
	return "union member not present"
}

type PreConditionFailedError struct {
	message string
}

func NewPreConditionFailedError(message string) *PreConditionFailedError {
	return &PreConditionFailedError{message: message}
}

func (e *PreConditionFailedError) Error() string {
	if e.message != "" {
		return e.message
	}
	return "precondition failed"
}
