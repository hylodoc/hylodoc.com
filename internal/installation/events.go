package installation

import (
	"encoding/json"
	"fmt"
)

func eventToJSON(event interface{}) (string, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("error marshalling event: %w", err)
	}
	return string(data), nil
}

/* InstallationEvent */

type InstallationPayload struct {
	Action       string       `json:"action"`
	Installation Installation `json:"installation"`
	Sender       Sender       `json:"sender"`
}

type Installation struct {
	ID int64 `json:"id"`
}

type Sender struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

/* RepositoriesResponse */

type RepositoriesResponse struct {
	TotalCount   int          `json:"total_count"`
	Repositories []Repository `json:"repositories"`
}

type Repository struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	HtmlUrl string `json:"html_url"`
	Owner   User   `json:"owner"`
}

type User struct {
	Login string `json:"login"`
}

type Organization struct {
	Login string `json:"login"`
}
