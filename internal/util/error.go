package util

type UserError struct {
	Message string /* message for user */
	Code    int    /* HTTP status code (optional) */
}

/* implements the error interface */
func (e UserError) Error() string {
	return e.Message
}
