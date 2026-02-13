package gitcred

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Request represents a parsed git credential helper request.
type Request struct {
	Protocol string
	Host     string
	Path     string
}

// Parse reads key=value lines from r until a blank line or EOF.
func Parse(r io.Reader) (*Request, error) {
	req := &Request{}
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		switch key {
		case "protocol":
			req.Protocol = value
		case "host":
			req.Host = value
		case "path":
			req.Path = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read credential input: %w", err)
	}
	return req, nil
}

// WriteResponse writes a git credential helper response.
func WriteResponse(w io.Writer, username, password string, expiryUTC int64) error {
	_, err := fmt.Fprintf(w, "username=%s\npassword=%s\npassword_expiry_utc=%d\n", username, password, expiryUTC)
	return err
}
