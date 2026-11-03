package service

import (
	"os"
	"testing"

	"github.com/op/go-logging"

	xuilogger "github.com/mhsanaei/3x-ui/v3/internal/logger"
)

func TestMain(m *testing.M) {
	// Code under test logs; the package logger must exist before any test reaches those paths.
	xuilogger.InitLogger(logging.ERROR)
	os.Exit(m.Run())
}
