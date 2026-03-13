package glimt

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// prefix is the marker used in .sql files to identify named queries.
const prefix = "-- :name "

// parse reads SQL queries from an io.Reader
// It expects queries to be defined in the format: -- :name QueryName
// Each query's SQL is collected until the next query name is encountered or EOF is reached.
func parse(r io.Reader) (map[string]string, error) {
	queries := make(map[string]string)

	var (
		currentName string
		currentSQL  strings.Builder
	)

	reader := bufio.NewReader(r)

	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, prefix) {
			// If we were building a query, save it before starting a new one
			if currentName != "" {
				sql := strings.TrimSpace(currentSQL.String())

				err := flushQuery(queries, currentName, sql)
				if err != nil {
					return nil, err
				}

				currentSQL.Reset()
			}

			currentName = strings.TrimSpace(line[len(prefix):])

			continue
		}

		if currentName != "" {
			currentSQL.WriteString(line)
			currentSQL.WriteByte('\n')
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}
	}

	if currentName != "" {
		sql := strings.TrimSpace(currentSQL.String())

		err := flushQuery(queries, currentName, sql)
		if err != nil {
			return nil, err
		}
	}

	return queries, nil
}

// flushQuery adds a query to the queries map, ensuring there are no duplicate names.
// It also checks that the query body is not empty.
func flushQuery(queries map[string]string, name, sql string) error {
	if !isValidName(name) {
		return fmt.Errorf("invalid query name %q", name)
	}

	if sql == "" {
		return fmt.Errorf("query %q has empty body", name)
	}

	if _, exists := queries[name]; exists {
		return fmt.Errorf("duplicate query name %q", name)
	}

	sql = strings.TrimRight(sql, ";")
	queries[name] = sql

	return nil
}

// isValidName checks if a query name is valid (consists of letters, digits,
// and underscores, and does not start with a digit).
func isValidName(name string) bool {
	for i, c := range name {
		if !unicode.IsLetter(c) && c != '_' && (i == 0 || !unicode.IsDigit(c)) {
			return false
		}
	}
	return true
}
