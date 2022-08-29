package sd_util

type Operator interface {
	Perform(leftVal, rightVal interface{}) bool
}

type EqualOperator struct {

}

func (o *EqualOperator) Perform() bool {
	return false
}