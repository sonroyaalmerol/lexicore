package starlarklib

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	_ "modernc.org/sqlite"
)

func TestHTTPModule(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message": "success"}`))
		case "/post":
			w.WriteHeader(http.StatusCreated)
			body := make([]byte, r.ContentLength)
			r.Body.Read(body)
			w.Write([]byte(`{"received": "` + string(body) + `"}`))
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "server error"}`))
		}
	}))
	defer server.Close()

	script := `
def test_http_get():
    resp = http.get("` + server.URL + `/get")
    if not resp["ok"]:
        return "GET request failed"
    if resp["status_code"] != 200:
        return "Expected status 200, got " + str(resp["status_code"])
    return None

def test_http_post():
    data = "test data"
    resp = http.post("` + server.URL + `/post", data=data)
    if not resp["ok"]:
        return "POST request failed"
    return None

def test_http_error():
    resp = http.get("` + server.URL + `/error")
    if resp["ok"]:
        return "Expected error response to not be ok"
    if resp["status_code"] != 500:
        return "Expected status 500, got " + str(resp["status_code"])
    return None

def test_http_client():
    client = http.client(timeout=60, insecure_skip_verify=True)
    if not client:
        return "Failed to create HTTP client"
    return None

def run_tests():
    tests = [
        ("HTTP GET", test_http_get),
        ("HTTP POST", test_http_post),
        ("HTTP Error Handling", test_http_error),
        ("HTTP Client Creation", test_http_client),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("HTTP test failed: %v", err)
	}

	printTestResults(t, results)
}

func TestJSONModule(t *testing.T) {
	script := `
def test_json_encode():
    data = {"name": "test", "value": 123, "nested": {"key": "value"}}
    encoded = json.encode(data)
    if not encoded:
        return "JSON encode returned empty"
    return None

def test_json_decode():
    json_str = '{"name": "test", "value": 123}'
    decoded = json.decode(json_str)
    if decoded["name"] != "test":
        return "Expected name='test', got " + decoded["name"]
    if decoded["value"] != 123:
        return "Expected value=123, got " + str(decoded["value"])
    return None

def test_json_roundtrip():
    original = {"key": "value", "number": 42, "array": [1, 2, 3]}
    encoded = json.encode(original)
    decoded = json.decode(encoded)
    
    if decoded["key"] != original["key"]:
        return "Roundtrip failed for string"
    if decoded["number"] != original["number"]:
        return "Roundtrip failed for number"
    if len(decoded["array"]) != len(original["array"]):
        return "Roundtrip failed for array"
    
    return None

def test_json_indent():
    data = {"name": "test"}
    encoded = json.encode(data, indent=2)
    if "\n" not in encoded:
        return "Indented JSON should contain newlines"
    return None

def run_tests():
    tests = [
        ("JSON Encode", test_json_encode),
        ("JSON Decode", test_json_decode),
        ("JSON Roundtrip", test_json_roundtrip),
        ("JSON Indent", test_json_indent),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("JSON test failed: %v", err)
	}

	printTestResults(t, results)
}

func TestBase64Module(t *testing.T) {
	script := `
def test_base64_encode():
    data = "Hello, World!"
    encoded = base64.encode(data)
    expected = "SGVsbG8sIFdvcmxkIQ=="
    if encoded != expected:
        return "Expected " + expected + ", got " + encoded
    return None

def test_base64_decode():
    encoded = "SGVsbG8sIFdvcmxkIQ=="
    decoded = base64.decode(encoded)
    expected = "Hello, World!"
    if decoded != expected:
        return "Expected " + expected + ", got " + decoded
    return None

def test_base64_roundtrip():
    original = "Test data with special chars: !@#$%^&*()"
    encoded = base64.encode(original)
    decoded = base64.decode(encoded)
    if decoded != original:
        return "Roundtrip failed: " + decoded
    return None

def run_tests():
    tests = [
        ("Base64 Encode", test_base64_encode),
        ("Base64 Decode", test_base64_decode),
        ("Base64 Roundtrip", test_base64_roundtrip),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("Base64 test failed: %v", err)
	}

	printTestResults(t, results)
}

func TestTimeModule(t *testing.T) {
	script := `
def test_time_now():
    now = time.now()
    if not now:
        return "time.now() returned empty"
    return None

def test_time_unix():
    timestamp = time.unix()
    if timestamp <= 0:
        return "Invalid unix timestamp: " + str(timestamp)
    return None

def test_time_parse():
    layout = "2006-01-02"
    value = "2024-01-15"
    timestamp = time.parse(layout, value)
    if timestamp <= 0:
        return "Failed to parse time"
    return None

def test_time_format():
    timestamp = 1705276800  # 2024-01-15 00:00:00 UTC
    layout = "2006-01-02"
    formatted = time.format(timestamp, layout)
    if "2024-01-15" not in formatted:
        return "Time format failed: " + formatted
    return None

def run_tests():
    tests = [
        ("Time Now", test_time_now),
        ("Time Unix", test_time_unix),
        ("Time Parse", test_time_parse),
        ("Time Format", test_time_format),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("Time test failed: %v", err)
	}

	printTestResults(t, results)
}

func TestHashModule(t *testing.T) {
	script := `
def test_hash_md5():
    data = "test"
    hash_val = hash.md5(data)
    expected = "098f6bcd4621d373cade4e832627b4f6"
    if hash_val != expected:
        return "Expected " + expected + ", got " + hash_val
    return None

def test_hash_sha1():
    data = "test"
    hash_val = hash.sha1(data)
    expected = "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"
    if hash_val != expected:
        return "Expected " + expected + ", got " + hash_val
    return None

def test_hash_sha256():
    data = "test"
    hash_val = hash.sha256(data)
    expected = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
    if hash_val != expected:
        return "Expected " + expected + ", got " + hash_val
    return None

def test_hash_sha512():
    data = "test"
    hash_val = hash.sha512(data)
    if len(hash_val) != 128:  # SHA512 produces 128 hex characters
        return "SHA512 hash should be 128 characters, got " + str(len(hash_val))
    return None

def run_tests():
    tests = [
        ("Hash MD5", test_hash_md5),
        ("Hash SHA1", test_hash_sha1),
        ("Hash SHA256", test_hash_sha256),
        ("Hash SHA512", test_hash_sha512),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("Hash test failed: %v", err)
	}

	printTestResults(t, results)
}

func TestSQLModule(t *testing.T) {
	// Create temporary SQLite database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize test database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			username TEXT NOT NULL,
			age INTEGER
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	db.Close()

	script := `
db_path = "` + dbPath + `"

def test_sql_connect():
    db = sql.sqlite(path=db_path)
    if not db:
        return "Failed to connect to database"
    db.close()
    return None

def test_sql_exec():
    db = sql.sqlite(path=db_path)
    result = db.exec(
        "INSERT INTO users (email, username, age) VALUES (?, ?, ?)",
        ["test@example.com", "testuser", 25]
    )
    db.close()
    
    if result["rows_affected"] != 1:
        return "Expected 1 row affected, got " + str(result["rows_affected"])
    return None

def test_sql_query():
    db = sql.sqlite(path=db_path)
    rows = db.query("SELECT * FROM users WHERE email = ?", ["test@example.com"])
    db.close()
    
    if len(rows) != 1:
        return "Expected 1 row, got " + str(len(rows))
    
    row = rows[0]
    if row["email"] != "test@example.com":
        return "Expected email test@example.com, got " + row["email"]
    if row["username"] != "testuser":
        return "Expected username testuser, got " + row["username"]
    if row["age"] != 25:
        return "Expected age 25, got " + str(row["age"])
    
    return None

def test_sql_query_row():
    db = sql.sqlite(path=db_path)
    row = db.query_row("SELECT * FROM users WHERE email = ?", ["test@example.com"])
    db.close()
    
    if not row:
        return "Expected a row, got None"
    if row["username"] != "testuser":
        return "Expected username testuser, got " + row["username"]
    
    return None

def test_sql_transaction():
    db = sql.sqlite(path=db_path)
    tx = db.begin()
    
    tx.exec(
        "INSERT INTO users (email, username, age) VALUES (?, ?, ?)",
        ["tx@example.com", "txuser", 30]
    )
    
    tx.commit()
    
    # Verify
    row = db.query_row("SELECT * FROM users WHERE email = ?", ["tx@example.com"])
    db.close()
    
    if not row:
        return "Transaction commit failed"
    if row["username"] != "txuser":
        return "Expected username txuser, got " + row["username"]
    
    return None

def test_sql_rollback():
    db = sql.sqlite(path=db_path)
    tx = db.begin()
    
    tx.exec(
        "INSERT INTO users (email, username, age) VALUES (?, ?, ?)",
        ["rollback@example.com", "rollbackuser", 35]
    )
    
    tx.rollback()
    
    # Verify it was rolled back
    row = db.query_row("SELECT * FROM users WHERE email = ?", ["rollback@example.com"])
    db.close()
    
    if row:
        return "Transaction rollback failed, row still exists"
    
    return None

def test_sql_ping():
    db = sql.sqlite(path=db_path)
    db.ping()
    db.close()
    return None

def run_tests():
    tests = [
        ("SQL Connect", test_sql_connect),
        ("SQL Exec", test_sql_exec),
        ("SQL Query", test_sql_query),
        ("SQL Query Row", test_sql_query_row),
        ("SQL Transaction", test_sql_transaction),
        ("SQL Rollback", test_sql_rollback),
        ("SQL Ping", test_sql_ping),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("SQL test failed: %v", err)
	}

	printTestResults(t, results)
}

func TestPrintFunction(t *testing.T) {
	script := `
def test_print():
    print("Test message")
    print("Multiple", "arguments", sep="-")
    return None

def run_tests():
    tests = [
        ("Print Function", test_print),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("Print test failed: %v", err)
	}

	printTestResults(t, results)
}

func TestIntegration(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"message": "success",
			"data":    []string{"item1", "item2", "item3"},
		})
	}))
	defer server.Close()

	// Create test SQLite database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "integration.db")

	script := `
server_url = "` + server.URL + `"
db_path = "` + dbPath + `"

def test_http_json_sql_integration():
    # Initialize database
    db = sql.sqlite(path=db_path)
    db.exec("""
        CREATE TABLE api_responses (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            message TEXT,
            data TEXT,
            checksum TEXT,
            created_at INTEGER
        )
    """)
    
    # Fetch data from HTTP API
    resp = http.get(server_url)
    if not resp["ok"]:
        return "HTTP request failed"
    
    # Parse JSON response
    body = json.decode(resp["body"])
    message = body["message"]
    data = body["data"]
    
    # Calculate checksum
    data_str = json.encode(data)
    checksum = hash.sha256(data_str)
    
    # Get current timestamp
    timestamp = time.unix()
    
    # Store in database
    result = db.exec(
        "INSERT INTO api_responses (message, data, checksum, created_at) VALUES (?, ?, ?, ?)",
        [message, data_str, checksum, timestamp]
    )
    
    if result["rows_affected"] != 1:
        return "Failed to insert into database"
    
    # Verify data was stored
    row = db.query_row("SELECT * FROM api_responses WHERE id = ?", [result["last_insert_id"]])
    
    if row["message"] != message:
        return "Message mismatch"
    
    if row["checksum"] != checksum:
        return "Checksum mismatch"
    
    # Encode result as base64
    final_data = base64.encode(row["data"])
    
    db.close()
    
    print("Integration test completed successfully")
    print("Final data (base64):", final_data)
    
    return None

def run_tests():
    tests = [
        ("HTTP + JSON + SQL + Hash + Time + Base64 Integration", test_http_json_sql_integration),
    ]
    
    results = []
    for name, test in tests:
        result = test()
        if result:
            results.append(name + ": FAILED - " + result)
        else:
            results.append(name + ": PASSED")
    
    return results
`

	results, err := executeStarlarkTest(t, script, "run_tests")
	if err != nil {
		t.Fatalf("Integration test failed: %v", err)
	}

	printTestResults(t, results)
}

// Helper functions

func executeStarlarkTest(t *testing.T, script, funcName string) (*starlark.List, error) {
	ctx := context.Background()
	thread := &starlark.Thread{Name: "test"}

	predeclared := MakeBuiltins(ctx)

	opts := &syntax.FileOptions{
		Set:             true,
		GlobalReassign:  true,
		TopLevelControl: true,
		Recursion:       true,
	}

	globals, err := starlark.ExecFileOptions(opts, thread, "test.star", []byte(script), predeclared)
	if err != nil {
		return nil, err
	}

	testFunc, ok := globals[funcName]
	if !ok {
		return nil, err
	}

	callable, ok := testFunc.(starlark.Callable)
	if !ok {
		return nil, err
	}

	result, err := starlark.Call(thread, callable, nil, nil)
	if err != nil {
		return nil, err
	}

	list, ok := result.(*starlark.List)
	if !ok {
		return nil, err
	}

	return list, nil
}

func printTestResults(t *testing.T, results *starlark.List) {
	for i := 0; i < results.Len(); i++ {
		result := results.Index(i)
		resultStr := result.String()
		// Remove quotes from string
		if len(resultStr) > 2 && resultStr[0] == '"' {
			resultStr = resultStr[1 : len(resultStr)-1]
		}

		t.Log(resultStr)

		if contains(resultStr, "FAILED") {
			t.Errorf("Test failed: %s", resultStr)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			anySubstring(s, substr))
}

func anySubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
