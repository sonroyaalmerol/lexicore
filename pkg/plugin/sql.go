package starlarklib

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/microsoft/go-mssqldb"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	_ "modernc.org/sqlite"
)

func makeSQLModule(ctx context.Context) *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "sql",
		Members: starlark.StringDict{
			"connect":  starlark.NewBuiltin("sql.connect", sqlConnect),
			"postgres": starlark.NewBuiltin("sql.postgres", sqlPostgres),
			"mysql":    starlark.NewBuiltin("sql.mysql", sqlMySQL),
			"sqlite":   starlark.NewBuiltin("sql.sqlite", sqlSQLite),
			"mssql":    starlark.NewBuiltin("sql.mssql", sqlMSSQL),
		},
	}
}

type sqlConn struct {
	db     *sql.DB
	driver string
}

func (s *sqlConn) String() string        { return fmt.Sprintf("sql.Connection<%s>", s.driver) }
func (s *sqlConn) Type() string          { return "sql.Connection" }
func (s *sqlConn) Freeze()               {}
func (s *sqlConn) Truth() starlark.Bool  { return starlark.True }
func (s *sqlConn) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: sql.Connection") }

func (s *sqlConn) Attr(name string) (starlark.Value, error) {
	switch name {
	case "query":
		return starlark.NewBuiltin("query", s.query), nil
	case "exec":
		return starlark.NewBuiltin("exec", s.exec), nil
	case "query_row":
		return starlark.NewBuiltin("query_row", s.queryRow), nil
	case "begin":
		return starlark.NewBuiltin("begin", s.begin), nil
	case "close":
		return starlark.NewBuiltin("close", s.close), nil
	case "ping":
		return starlark.NewBuiltin("ping", s.ping), nil
	}
	return nil, nil
}

func (s *sqlConn) AttrNames() []string {
	return []string{"query", "exec", "query_row", "begin", "close", "ping"}
}

// Generic connect function
func sqlConnect(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var driver, dsn string
	var maxOpenConns, maxIdleConns int = 10, 5

	if err := starlark.UnpackArgs(
		"sql.connect",
		args,
		kwargs,
		"driver", &driver,
		"dsn", &dsn,
		"max_open_conns?", &maxOpenConns,
		"max_idle_conns?", &maxIdleConns,
	); err != nil {
		return nil, err
	}

	return openDatabase(driver, dsn, maxOpenConns, maxIdleConns)
}

// PostgreSQL convenience function
func sqlPostgres(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var host, user, password, dbname string
	var port int = 5432
	var sslmode string = "disable"
	var maxOpenConns, maxIdleConns int = 10, 5

	if err := starlark.UnpackArgs(
		"sql.postgres",
		args,
		kwargs,
		"host", &host,
		"user", &user,
		"password", &password,
		"dbname", &dbname,
		"port?", &port,
		"sslmode?", &sslmode,
		"max_open_conns?", &maxOpenConns,
		"max_idle_conns?", &maxIdleConns,
	); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)

	return openDatabase("postgres", dsn, maxOpenConns, maxIdleConns)
}

// MySQL convenience function
func sqlMySQL(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var host, user, password, dbname string
	var port int = 3306
	var charset string = "utf8mb4"
	var parseTime bool = true
	var maxOpenConns, maxIdleConns int = 10, 5

	if err := starlark.UnpackArgs(
		"sql.mysql",
		args,
		kwargs,
		"host", &host,
		"user", &user,
		"password", &password,
		"dbname", &dbname,
		"port?", &port,
		"charset?", &charset,
		"parse_time?", &parseTime,
		"max_open_conns?", &maxOpenConns,
		"max_idle_conns?", &maxIdleConns,
	); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t",
		user, password, host, port, dbname, charset, parseTime,
	)

	return openDatabase("mysql", dsn, maxOpenConns, maxIdleConns)
}

// SQLite convenience function
func sqlSQLite(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var path string
	var mode string = "rwc" // read-write-create

	if err := starlark.UnpackArgs(
		"sql.sqlite",
		args,
		kwargs,
		"path", &path,
		"mode?", &mode,
	); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf("file:%s?mode=%s", path, mode)
	return openDatabase("sqlite", dsn, 1, 1) // SQLite doesn't benefit from connection pooling
}

// Microsoft SQL Server convenience function
func sqlMSSQL(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var host, user, password, database string
	var port int = 1433
	var encrypt string = "disable"
	var maxOpenConns, maxIdleConns int = 10, 5

	if err := starlark.UnpackArgs(
		"sql.mssql",
		args,
		kwargs,
		"host", &host,
		"user", &user,
		"password", &password,
		"database", &database,
		"port?", &port,
		"encrypt?", &encrypt,
		"max_open_conns?", &maxOpenConns,
		"max_idle_conns?", &maxIdleConns,
	); err != nil {
		return nil, err
	}

	dsn := fmt.Sprintf(
		"server=%s;port=%d;user id=%s;password=%s;database=%s;encrypt=%s",
		host, port, user, password, database, encrypt,
	)

	return openDatabase("sqlserver", dsn, maxOpenConns, maxIdleConns)
}

func openDatabase(driver, dsn string, maxOpenConns, maxIdleConns int) (starlark.Value, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &sqlConn{db: db, driver: driver}, nil
}

func (s *sqlConn) query(
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

	// Convert params to interface slice
	var queryArgs []any
	if params != nil {
		queryArgs = make([]any, params.Len())
		for i := 0; i < params.Len(); i++ {
			queryArgs[i] = starlarkToGoValue(params.Index(i))
		}
	}

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return rowsToStarlark(rows)
}

func (s *sqlConn) exec(
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

	result, err := s.db.Exec(query, queryArgs...)
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

func (s *sqlConn) queryRow(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var query string
	var params *starlark.List

	if err := starlark.UnpackArgs(
		"query_row",
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

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query_row failed: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return starlark.None, nil
	}

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	values := make([]any, len(columns))
	valuePtrs := make([]any, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	rowDict := starlark.NewDict(len(columns))
	for i, col := range columns {
		rowDict.SetKey(starlark.String(col), goValueToStarlark(values[i]))
	}

	return rowDict, nil
}

func (s *sqlConn) begin(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction failed: %w", err)
	}

	return &sqlTx{tx: tx, driver: s.driver}, nil
}

func (s *sqlConn) ping(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := s.db.Ping()
	if err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}
	return starlark.None, nil
}

func (s *sqlConn) close(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := s.db.Close()
	if err != nil {
		return nil, fmt.Errorf("close failed: %w", err)
	}
	return starlark.None, nil
}

func rowsToStarlark(rows *sql.Rows) (starlark.Value, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []starlark.Value

	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		rowDict := starlark.NewDict(len(columns))
		for i, col := range columns {
			rowDict.SetKey(starlark.String(col), goValueToStarlark(values[i]))
		}

		results = append(results, rowDict)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return starlark.NewList(results), nil
}
