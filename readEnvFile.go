package httpmin

import (
	"os"
	"strings"
)

func readEnvFile(path string) {
	bytes, err := os.ReadFile(path)

	if err != nil {
		return
	}

	for line := range strings.Lines(string(bytes)) {
		line = strings.TrimSpace(line)
		key, value, lineOk := strings.Cut(line, "=")

		if !lineOk {
			continue
		}

		_, keyExists := os.LookupEnv(key)

		if keyExists {
			continue
		}

		os.Setenv(key, value)
	}
}

func envOrDefault(key, defaultValue string) string {
	value, ok := os.LookupEnv(key)

	if ok {
		return value
	}

	return defaultValue
}
