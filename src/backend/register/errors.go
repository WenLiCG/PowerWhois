package register

import "errors"

var (
	ErrorInvalidRegisterType        = errors.New("invalid register type")
	ErrorCustomizeApiRegisterResult = errors.New("customize api register result error")
)
