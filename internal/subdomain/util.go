package subdomain

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"
)

func GenerateDemoSubdomain() string {
	now := time.Now().UnixNano()

	hash := md5.New()
	hash.Write([]byte(fmt.Sprintf("%d", now)))
	hashString := hex.EncodeToString(hash.Sum(nil))
	/* use first 11 character */
	return hashString[:11]
}
