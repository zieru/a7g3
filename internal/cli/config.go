// Package cli handles command-line argument parsing, configuration, and validation for g3a.
package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DefaultConfigFileName is the standard configuration file name in the user's home profile.
const DefaultConfigFileName = ".g3a.config"

// LoadConfig loads alias mappings from the configuration file.
// It searches in order:
// 1. Environment variable G3A_CONFIG (if set)
// 2. User's home directory (e.g. /home/user/.g3a.config or C:\Users\user\.g3a.config)
// 3. Current working directory (.g3a.config)
//
// If no configuration file is found, it returns an empty map and no error.
func LoadConfig() (map[string]string, string, error) {
	paths := candidateConfigPaths()
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			aliases, err := ParseConfigFile(p)
			return aliases, p, err
		}
	}
	return make(map[string]string), "", nil
}

// candidateConfigPaths returns candidate file paths for the config file in priority order.
func candidateConfigPaths() []string {
	var candidates []string

	// 1. Custom env var
	if envPath := os.Getenv("G3A_CONFIG"); envPath != "" {
		candidates = append(candidates, envPath)
	}

	// 2. User home profile
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, DefaultConfigFileName))
	}

	// 3. Current working directory
	candidates = append(candidates, DefaultConfigFileName)

	return candidates
}

// ParseConfigFile parses a .g3a.config file and returns alias -> filepath mapping.
// Supported line formats:
//   funneling /path/to/file.csv
//   funneling = /path/to/file.parquet
//   funnelingx "C:\Path With Spaces\file.csv"
// Lines starting with #, ;, or // are ignored as comments.
func ParseConfigFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	aliases := make(map[string]string)
	scanner := bufio.NewScanner(file)

	homeDir, _ := os.UserHomeDir()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") {
			continue
		}

		key, val := parseConfigLine(line)
		if key != "" && val != "" {
			// Expand ~/ or ~\
			if homeDir != "" && (strings.HasPrefix(val, "~/") || strings.HasPrefix(val, "~\\")) {
				val = filepath.Join(homeDir, val[2:])
			}
			aliases[key] = val
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return aliases, nil
}

// parseConfigLine extracts the alias key and target file path from a single line.
func parseConfigLine(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}

	// Support key=val or key = val
	var key, val string
	if eqIdx := strings.Index(line, "="); eqIdx != -1 {
		key = strings.TrimSpace(line[:eqIdx])
		val = strings.TrimSpace(line[eqIdx+1:])
	} else {
		// Split by first whitespace (space or tab)
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			key = parts[0]
			// Value is everything after key (preserving inner spaces if not quoted or unquoted)
			rest := strings.TrimSpace(line[len(key):])
			val = rest
		} else if len(parts) == 1 {
			return parts[0], ""
		}
	}

	// Strip surrounding quotes if present ("..." or '...')
	if len(val) >= 2 {
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			val = val[1 : len(val)-1]
		}
	}

	return key, strings.TrimSpace(val)
}
