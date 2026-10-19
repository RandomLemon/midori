package ui

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("LANG", "en_US.UTF-8")
	_ = os.Setenv("LC_ALL", "en_US.UTF-8")
	_ = os.Setenv("LANGUAGE", "en")
	os.Exit(m.Run())
}
