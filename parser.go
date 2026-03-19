package glimt

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// queryPrefix is the marker used in .sql files to identify named queries.
const queryPrefix = "-- :name "

// parser reads SQL queries from an io.Reader and organizes them by name.
type parser struct {
	queries map[string]string
}

// newParser creates a new parser instance with an initialized queries map.
func newParser() *parser {
	return &parser{queries: make(map[string]string)}
}

// parse reads SQL queries from an io.Reader
// It expects queries to be defined in the format: -- :name QueryName
func (p *parser) parse(r io.Reader) error {
	scanner := bufio.NewScanner(r)

	var (
		currentName string
		currentSQL  strings.Builder
	)

	flush := func() error {
		if currentName == "" {
			return nil
		}

		return p.flush(currentName, currentSQL.String())
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check if the line starts with the query name prefix
		if strings.HasPrefix(line, queryPrefix) {
			// If we were building a query, save it before starting a new one
			if err := flush(); err != nil {
				return err
			}

			// Start a new query
			currentName = strings.TrimSpace(line[len(queryPrefix):])
			currentSQL.Reset()
			continue
		}

		// Skip comment lines that are not query names
		if strings.HasPrefix(line, "--") {
			continue
		}

		// If we're currently building a query, append the line to the SQL
		if currentName != "" {
			currentSQL.WriteString(line)
			currentSQL.WriteByte('\n')
		}
	}

	// After the loop, check if there's an unfinished query to flush
	if err := flush(); err != nil {
		return err
	}

	return scanner.Err()
}

// flush adds a query to the queries map, ensuring there are no duplicate names.
// It also checks that the query body is not empty.
func (p *parser) flush(name, sql string) error {
	sql = p.cleanSQL(sql)

	if !validName(name) {
		return fmt.Errorf("invalid query name %q", name)
	}
	if sql == "" {
		return fmt.Errorf("query %q has empty body", name)
	}
	if _, exists := p.queries[name]; exists {
		return fmt.Errorf("duplicate query name %q", name)
	}

	p.queries[name] = sql
	return nil
}

// cleanSQL removes comments and trims whitespace from a SQL string.
func (p *parser) cleanSQL(sql string) string {
	sql = p.stripBlockComments(sql)

	lines := strings.Split(sql, "\n")
	var cleaned []string

	for _, line := range lines {
		line = p.stripInlineComments(line)
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	result := strings.Join(cleaned, "\n")
	return strings.TrimRight(result, ";")
}

// stripInlineComments removes any inline comments from a line of SQL.
func (p *parser) stripInlineComments(line string) string {
	before, _, ok := strings.Cut(line, "--")
	if !ok {
		return line
	}
	return before
}

// stripBlockComments removes block comments that span multiple lines.
func (p *parser) stripBlockComments(sql string) string {
	for {
		start := strings.Index(sql, "/*")
		if start == -1 {
			break
		}
		end := strings.Index(sql[start:], "*/")
		if end == -1 {
			// unclosed block comment — strip from /* to end
			return strings.TrimSpace(sql[:start])
		}
		sql = sql[:start] + sql[start+end+2:]
	}
	return sql
}
