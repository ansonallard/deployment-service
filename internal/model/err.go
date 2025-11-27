package model

type unionMemberNotPresent struct{}

func (u *unionMemberNotPresent) Error() string {
	return "union member not present"
}
