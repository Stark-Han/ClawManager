package systembackup

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"clawreef/internal/systembackup/team"
)

// VerifyMySQL only issues SELECTs, using a read-only repeatable-read transaction.
// expected must describe the normalized copy, not the unmodified source archive.
// The caller supplies a separate SELECT-only credential; this code cannot turn
// a privileged restore account into a verifier account.
func VerifyMySQL(ctx context.Context, db *sql.DB, expected io.Reader, maxRows int64) (SQLResult, error) {
	if maxRows < 1 {
		return SQLResult{}, errors.New("verification budget required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return SQLResult{}, err
	}
	defer tx.Rollback()
	var checked int64
	var current SQLTable
	var count int64
	finish := func() error {
		if current.Name == "" {
			return nil
		}
		q, _ := QuoteIdentifier(current.Name)
		var actual int64
		if e := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+q).Scan(&actual); e != nil {
			return e
		}
		if actual != count {
			return fmt.Errorf("row count mismatch: %s", current.Name)
		}
		return nil
	}
	result, err := ReadSQLSnapshot(expected, func(table SQLTable) error {
		if e := finish(); e != nil {
			return e
		}
		actual, e := ReadTableSchema(ctx, tx, table.Name)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(table, actual) {
			return fmt.Errorf("schema mismatch: %s", table.Name)
		}
		current = table
		count = 0
		return nil
	}, func(table SQLTable, row team.Row) error {
		checked++
		count++
		if checked > maxRows {
			return errors.New("verification row budget exceeded")
		}
		var predicates []string
		var args []any
		for _, pk := range table.PrimaryKey {
			c, ok := row[pk]
			if !ok || c.IsNull {
				return errors.New("invalid primary key")
			}
			q, e := QuoteIdentifier(pk)
			if e != nil {
				return e
			}
			predicates = append(predicates, q+"=?")
			args = append(args, nonNullSQLBytes(c.Data))
		}
		if len(predicates) == 0 {
			return errors.New("missing primary key")
		}
		q, _ := QuoteIdentifier(table.Name)
		cells := make([]any, len(table.Columns))
		targets := make([]any, len(cells))
		for i := range cells {
			targets[i] = &cells[i]
		}
		if e := tx.QueryRowContext(ctx, "SELECT * FROM "+q+" WHERE "+strings.Join(predicates, " AND "), args...).Scan(targets...); e != nil {
			return errors.New("expected row unavailable")
		}
		for i, c := range table.Columns {
			actual, e := sqlCell(cells[i], c)
			if e != nil {
				return e
			}
			want := row[c.Name]
			if want.IsNull != actual.IsNull || !bytes.Equal(want.Data, actual.Data) {
				return fmt.Errorf("value mismatch: %s.%s", table.Name, c.Name)
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if err = finish(); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
