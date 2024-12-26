package response

import "net/http"

type Response interface {
	Respond(http.ResponseWriter, *http.Request)
}
