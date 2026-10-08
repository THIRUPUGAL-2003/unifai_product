package handlers

import (
	"testing"

	"github.com/fasthttp/router"
	"github.com/stretchr/testify/require"
)

func TestBrowserAIRoutesRegistration(t *testing.T) {
	r := router.New()
	h := &BrowserAIHandler{}
	require.NotPanics(t, func() {
		h.RegisterRoutes(r)
	})
}
