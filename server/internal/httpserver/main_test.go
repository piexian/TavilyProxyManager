package httpserver

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMain sets gin to test mode once for the whole package.
// Individual tests must not call gin.SetMode themselves: it mutates a global
// and races with t.Parallel() tests under -race.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
