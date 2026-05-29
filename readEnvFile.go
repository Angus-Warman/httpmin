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
		line = strings.TrimPrefix(line, "export ")

		if len(line) == 0 || line[0] == '#' {
			continue
		}

		key, value, lineOk := strings.Cut(line, "=")

		if !lineOk {
			continue
		}

		key = strings.TrimSpace(key)

		if key == "" {
			continue
		}

		_, alreadySet := os.LookupEnv(key)

		if alreadySet {
			continue
		}

		value = strings.TrimSpace(value)
		value = stripInlineComment(value)
		value = unquote(value)

		os.Setenv(key, value)
	}
}

func stripInlineComment(original string) string {
	if len(original) > 0 && (original[0] == '"' || original[0] == '\'') {
		return original // let unquote handle it, comment may be inside quotes
	}

	before, _, ok := strings.Cut(original, " #")

	if !ok {
		return original
	}

	value := strings.TrimSpace(before)

	return value
}

func unquote(original string) string {
	if len(original) < 2 {
		return original
	}

	firstChar := original[0]

	if firstChar != '"' && firstChar != '\'' {
		return original // Not quoted
	}

	lastChar := original[len(original)-1]

	if firstChar != lastChar {
		return original // Badly quoted
	}

	value := original[1 : len(original)-1]

	if firstChar == '"' {
		value = strings.NewReplacer(
			`\n`, "\n",
			`\t`, "\t",
			`\r`, "\r",
			`\\`, "\\",
			`\"`, "\"",
		).Replace(value)
	}

	return value
}
