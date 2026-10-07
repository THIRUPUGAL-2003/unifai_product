package oauth2

import "github.com/gateway/gateway/core/schemas"

var logger schemas.Logger

func SetLogger(l schemas.Logger) {
	logger = l
}
