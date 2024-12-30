package util

type CustomError struct {
	Message string /* Message for user */
	Code    int    /* HTTP status Code (optional) */
}

func CreateCustomError(Message string, Code int) *CustomError {
	return &CustomError{
		Message: Message,
		Code:    Code,
	}
}

/* implements the error interface */
func (e *CustomError) Error() string {
	return e.Message
}
