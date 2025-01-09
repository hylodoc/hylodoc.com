package session

/* struct used to pass session info to pages */
type UserInfo struct {
	UserID       *string
	Email        *string
	Username     *string
	GithubLinked bool
	GithubEmail  *string
}

func ConvertSessionToUserInfo(sesh *Session) *UserInfo {
	return &UserInfo{
		UserID:       sesh.userID,
		Email:        sesh.email,
		Username:     sesh.username,
		GithubLinked: sesh.githubLinked,
		GithubEmail:  sesh.githubEmail,
	}
}
