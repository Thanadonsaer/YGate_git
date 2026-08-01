package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func LoadDefault() error {
	for _, name := range candidateFiles() {
		if err := load(name); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func candidateFiles() []string {
	if name := strings.TrimSpace(os.Getenv("YGATE_ENV_FILE")); name != "" {
		return []string{name}
	}
	return []string{
		".env",
		filepath.Join("deploy", "local", ".env"),
		filepath.Join("..", "..", "deploy", "local", ".env"),
	}
}

func load(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"'`))
	}
	return scanner.Err()
}
