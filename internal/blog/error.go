package blog

type customError struct {
	Message string /* Message for user */
	Code    int    /* HTTP status Code (optional) */
}

func createCustomError(Message string, Code int) *customError {
	return &customError{
		Message: Message,
		Code:    Code,
	}
}

/* implements the error interface */
func (e *customError) Error() string {
	return e.Message
}
