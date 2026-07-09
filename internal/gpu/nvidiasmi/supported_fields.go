package nvidiasmi

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
)

func supportedFields(runner execRunner, smiPath, helpFlag string) (map[string]bool, error) {
	out, err := runner.Run(context.Background(), smiPath, helpFlag)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.TrimPrefix(helpFlag, "--"), err)
	}
	return parseSupportedFields(out), nil
}

func parseSupportedFields(out []byte) map[string]bool {
	fields := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		field, ok := parseHelpField(scanner.Text())
		if ok {
			fields[field] = true
		}
	}
	return fields
}

func parseHelpField(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "\"") {
		return "", false
	}
	remainder := strings.TrimPrefix(trimmed, "\"")
	end := strings.Index(remainder, "\"")
	if end <= 0 {
		return "", false
	}
	return remainder[:end], true
}
