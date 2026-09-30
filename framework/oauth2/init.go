package oauth2

import "github.com/unifai/unifai/core/schemas"

var logger schemas.Logger

func SetLogger(l schemas.Logger) {
	logger = l
}
