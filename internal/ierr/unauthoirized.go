package ierr

type UnAuthorizedError struct{}

func (e *UnAuthorizedError) Error() string {
	return "Unauthorized"
}
