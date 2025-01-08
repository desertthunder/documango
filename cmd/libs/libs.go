// package libs contains helper functions and utilities used application wide.
package libs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/charmbracelet/log"
)

// type ErrorData is the serialized error response
type ErrorData struct {
	Status int    `json:"statusCode"`
	Err    string `json:"ErrorMessage"`
}

// function GenerateLogID generates a random 8 digit identifier for
// logs.
func GenerateLogID(logger *log.Logger) (string, error) {
	var id [8]byte
	_, err := rand.Read(id[:])

	if err != nil {
		logger.Errorf("error generating random ID: %v", err)
		return "", err
	}

	encoded := hex.EncodeToString(id[:])

	return encoded, nil
}

func CreateErrorJSON(status int, msg error) []byte {
	errData := ErrorData{status, msg.Error()}
	data, _ := json.Marshal(errData)
	return data
}

func IsNotMarkdown(n string) bool {
	p := strings.Split(n, ".")
	return p[len(p)-1] != "md"
}
