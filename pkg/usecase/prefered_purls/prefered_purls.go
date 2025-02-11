package preferedpurls

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func InitPurlMap(filename string) (map[string]bool, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	repoMap := make(map[string]bool)

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			repoMap[line] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return repoMap, nil
}
