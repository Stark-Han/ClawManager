package db

import (
	"embed"
	"fmt"
	"log"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/upper/db/v4"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

const schemaMigrationsTable = "schema_migrations"

func applyEmbeddedMigrations(session db.Session) error {
	if err := ensureSchemaMigrationsTable(session); err != nil {
		return err
	}

	applied, err := listAppliedMigrations(session)
	if err != nil {
		return err
	}

	entries, err := embeddedMigrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to list embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if _, ok := applied[entry.Name()]; ok {
			continue
		}

		rawSQL, err := embeddedMigrations.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read embedded migration %s: %w", entry.Name(), err)
		}

		statements := splitSQLStatements(string(rawSQL))
		for idx, statement := range statements {
			if _, err := session.SQL().Exec(statement); err != nil {
				return fmt.Errorf("failed to execute migration %s statement %d: %w", entry.Name(), idx+1, err)
			}
		}

		if _, err := session.SQL().Exec(
			fmt.Sprintf("INSERT INTO %s (filename, applied_at) VALUES (?, ?)", schemaMigrationsTable),
			entry.Name(),
			time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("failed to record applied migration %s: %w", entry.Name(), err)
		}

		log.Printf("Applied database migration %s", entry.Name())
	}

	return nil
}

func ensureSchemaMigrationsTable(session db.Session) error {
	_, err := session.SQL().Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INT AUTO_INCREMENT PRIMARY KEY,
			filename VARCHAR(255) NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE KEY uk_schema_migrations_filename (filename)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`, schemaMigrationsTable))
	if err != nil {
		return fmt.Errorf("failed to ensure schema migrations table: %w", err)
	}
	return nil
}

func listAppliedMigrations(session db.Session) (map[string]struct{}, error) {
	rows, err := session.SQL().Query(fmt.Sprintf("SELECT filename FROM %s", schemaMigrationsTable))
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]struct{})
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return nil, fmt.Errorf("failed to scan applied migration: %w", err)
		}
		applied[filename] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate applied migrations: %w", err)
	}

	return applied, nil
}

func splitSQLStatements(input string) []string {
	statements := make([]string, 0)
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(input); i++ {
		ch := input[i]
		var next byte
		if i+1 < len(input) {
			next = input[i+1]
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if inSingleQuote {
			current.WriteByte(ch)
			if ch == '\\' && next != 0 {
				i++
				current.WriteByte(next)
				continue
			}
			if ch == '\'' {
				if next == '\'' {
					i++
					current.WriteByte(next)
					continue
				}
				inSingleQuote = false
			}
			continue
		}

		if inDoubleQuote {
			current.WriteByte(ch)
			if ch == '\\' && next != 0 {
				i++
				current.WriteByte(next)
				continue
			}
			if ch == '"' {
				if next == '"' {
					i++
					current.WriteByte(next)
					continue
				}
				inDoubleQuote = false
			}
			continue
		}

		if inBacktick {
			current.WriteByte(ch)
			if ch == '`' {
				inBacktick = false
			}
			continue
		}

		if ch == '-' && next == '-' {
			var afterNext byte
			if i+2 < len(input) {
				afterNext = input[i+2]
			}
			if afterNext == 0 || afterNext == ' ' || afterNext == '\t' || afterNext == '\r' || afterNext == '\n' {
				inLineComment = true
				i++
				continue
			}
		}

		if ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}

		switch ch {
		case '\'':
			inSingleQuote = true
			current.WriteByte(ch)
		case '"':
			inDoubleQuote = true
			current.WriteByte(ch)
		case '`':
			inBacktick = true
			current.WriteByte(ch)
		case ';':
			statement := strings.TrimSpace(current.String())
			if statement != "" {
				statements = append(statements, statement)
			}
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}

	if statement := strings.TrimSpace(current.String()); statement != "" {
		statements = append(statements, statement)
	}

	return statements
}
