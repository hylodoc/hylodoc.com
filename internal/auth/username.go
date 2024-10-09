package auth

import (
	"time"

	"math/rand"
)

var (
	alphabet       = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	usernameLength = 11
)

func getRandomInt(maximum int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(maximum)
}

func GenerateUsername() string {
	username := make([]rune, usernameLength)
	for i := 0; i < usernameLength; i++ {
		username[i] = alphabet[getRandomInt(len(alphabet))]
	}
	return string(username)
}
