package installation

import (
	"encoding/json"
	"fmt"
)

/* InstallationEvent */

type InstallationEvent struct {
	Action       string       `json:"action"`
	Installation Installation `json:"installation"`
	Repositories []Repository `json:"repositories"`
	Sender       User         `json:"sender"`
}

/* InstallationRepositoriesEvent */

type InstallationRepositoriesEvent struct {
	Action              string       `json:"action"`
	Installation        Installation `json:"installation"`
	RepositoriesAdded   []Repository `json:"repositories_added"`
	RepositoriesRemoved []Repository `json:"repositories_removed"`
	Sender              User         `json:"sender"`
}

/* PushEvent */

type PushEvent struct {
	Installation Installation `json:"installation"`
	Ref          string       `json:"ref"`
	Before       string       `json:"before"`
	After        string       `json:"after"`
	Repository   Repository   `json:"repository"`
	Pusher       User         `json:"pusher"`
	Sender       User         `json:"sender"`
	Commits      []Commit     `json:"commits"`
	HeadCommit   Commit       `json:"head_commit"`
}

/* Common nested structs */

type User struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    User   `json:"owner"`
	HtmlUrl  string `json:"html_url"`
}

type Commit struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	URL       string `json:"url"`
	Author    struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
}

type Installation struct {
	ID                  int64  `json:"id"`
	Account             User   `json:"account"`
	RepositorySelection string `json:"repository_selection"`
	HtmlUrl             string `json:"html_url"`
	AppID               int64  `json:"app_id"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

func eventToJSON(event interface{}) (string, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("error marshalling event: %w", err)
	}
	return string(data), nil
}
