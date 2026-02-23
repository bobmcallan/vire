package data

import (
	"os"
	"testing"

	tcommon "github.com/bobmcallan/vire/tests/common"
)

func TestMain(m *testing.M) {
	code := m.Run()
	tcommon.CleanupSurrealDB()
	os.Exit(code)
}
