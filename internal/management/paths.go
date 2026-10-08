package management

import (
	"path"
	"strings"
	"unicode/utf8"

	"minecraft-hosting/internal/server"
)

// Path rejects alternate encodings and traversal instead of silently cleaning it.
// Paths always describe a container path and are never used with the host OS.
func Path(value string, rootAllowed bool) (string, error) {
	if len(value) == 0 || len(value) > 1024 || !utf8.ValidString(value) || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\%") {
		return "", server.ErrInvalid
	}
	for _, c := range value {
		if c < 32 || c == 127 {
			return "", server.ErrInvalid
		}
	}
	if value == "/" {
		if rootAllowed {
			return value, nil
		}
		return "", server.ErrInvalid
	}
	if strings.HasSuffix(value, "/") {
		return "", server.ErrInvalid
	}
	for _, v := range strings.Split(value[1:], "/") {
		if v == "" || v == "." || v == ".." {
			return "", server.ErrInvalid
		}
	}
	if path.Clean(value) != value {
		return "", server.ErrInvalid
	}
	return value, nil
}
func Name(value string) error {
	if strings.Contains(value, "/") || len(value) > 255 {
		return server.ErrInvalid
	}
	_, err := Path("/"+value, false)
	return err
}
