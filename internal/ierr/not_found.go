package ierr

type NotFoundError struct{}

func (e *NotFoundError) Error() string {
	return "NotFound"
}
