package response

import (
	"fmt"
	"net/http"
)

type csvresponse struct {
	name string
	b    []byte
}

func NewCsvFile(name string, b []byte) Response { return &csvresponse{name, b} }

func (csv *csvresponse) Respond(w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set(
		"Content-Disposition",
		fmt.Sprintf("attachment; filename=%s", csv.name),
	)
	_, err := w.Write(csv.b)
	return err
}
