package helpers

import (
	"fmt"
	"os"
	"strings"
)

func GetAPIURL() string {
	value, exists := os.LookupEnv("API_URL")
	value = strings.TrimSpace(value)
	if !exists || value == "" {
		panic(fmt.Errorf("API_URL is not set"))
	}
	return strings.TrimRight(value, "/")
}