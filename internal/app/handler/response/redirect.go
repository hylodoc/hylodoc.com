package response

import "net/http"

type redirect struct {
	url    string
	status int
}

func NewRedirect(url string, status int) Response {
	return &redirect{url, status}
}

func (redirect *redirect) Respond(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, redirect.url, redirect.status)
}
