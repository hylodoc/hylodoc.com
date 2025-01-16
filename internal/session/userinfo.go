package session

/* struct used to pass session info to pages */
type UserInfo struct {
	UserID   *string
	Email    *string
	Username *string

	IsErrorPage bool
}

func ConvertSessionToUserInfo(sesh *Session) *UserInfo {
	return &UserInfo{
		UserID:   sesh.userID,
		Email:    sesh.email,
		Username: sesh.username,
	}
}

func ConvertSessionToUserInfoError(sesh *Session) *UserInfo {
	return &UserInfo{
		UserID:      sesh.userID,
		Email:       sesh.email,
		Username:    sesh.username,
		IsErrorPage: true,
	}
}
