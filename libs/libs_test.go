package libs

import (
	"strconv"
	"testing"
)

func TestLibsPackage(t *testing.T) {
	logger := CreateConsoleLogger("[libs test]")

	t.Run("GenerateLogID", func(t *testing.T) {
		id := GenerateLogID(logger)
		if len(id) < 8 {
			t.Errorf("invalid length of id %v", id)
		}

		_, err := strconv.Atoi(id)

		if err == nil {
			t.Errorf("id shouldn't be an integer but it is")
		}
	})

	t.Run("CreateErrorJSON", func(t *testing.T) {
		t.Skip()
	})
}
