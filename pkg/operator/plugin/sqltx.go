package starlarklib

import (
	"database/sql"
	"fmt"

	"go.starlark.net/starlark"
)

type sqlTx struct {
	tx     *sql.Tx
	driver string
}

func (t *sqlTx) String() string        { return fmt.Sprintf("sql.Transaction<%s>", t.driver) }
func (t *sqlTx) Type() string          { return "sql.Transaction" }
func (t *sqlTx) Freeze()               {}
func (t *sqlTx) Truth() starlark.Bool  { return starlark.True }
func (t *sqlTx) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: sql.Transaction") }

func (t *sqlTx) Attr(name string) (starlark.Value, error) {
	switch name {
	case "query":
		return starlark.NewBuiltin("query", t.query), nil
	case "exec":
		return starlark.NewBuiltin("exec", t.exec), nil
	case "commit":
		return starlark.NewBuiltin("commit", t.commit), nil
	case "rollback":
		return starlark.NewBuiltin("rollback", t.rollback), nil
	}
	return nil, nil
}

func (t *sqlTx) AttrNames() []string {
	return []string{"query", "exec", "commit", "rollback"}
}

func (t *sqlTx) query(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var query string
	var params *starlark.List

	if err := starlark.UnpackArgs(
		"query",
		args,
		kwargs,
		"query", &query,
		"params?", &params,
	); err != nil {
		return nil, err
	}

	var queryArgs []any
	if params != nil {
		queryArgs = make([]any, params.Len())
		for i := 0; i < params.Len(); i++ {
			queryArgs[i] = starlarkToGoValue(params.Index(i))
		}
	}

	rows, err := t.tx.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return rowsToStarlark(rows)
}

func (t *sqlTx) exec(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var query string
	var params *starlark.List

	if err := starlark.UnpackArgs(
		"exec",
		args,
		kwargs,
		"query", &query,
		"params?", &params,
	); err != nil {
		return nil, err
	}

	var queryArgs []any
	if params != nil {
		queryArgs = make([]any, params.Len())
		for i := 0; i < params.Len(); i++ {
			queryArgs[i] = starlarkToGoValue(params.Index(i))
		}
	}

	result, err := t.tx.Exec(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec failed: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	lastInsertId, _ := result.LastInsertId()

	resultDict := starlark.NewDict(2)
	resultDict.SetKey(starlark.String("rows_affected"), starlark.MakeInt64(rowsAffected))
	resultDict.SetKey(starlark.String("last_insert_id"), starlark.MakeInt64(lastInsertId))

	return resultDict, nil
}

func (t *sqlTx) commit(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := t.tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit failed: %w", err)
	}
	return starlark.None, nil
}

func (t *sqlTx) rollback(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := t.tx.Rollback()
	if err != nil {
		return nil, fmt.Errorf("rollback failed: %w", err)
	}
	return starlark.None, nil
}
