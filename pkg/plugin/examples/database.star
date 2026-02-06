name = "database"

def initialize(config):
    """Initialize database connection."""
    db_type = config.get("db_type", "postgres")
    
    global _db
    
    if db_type == "postgres":
        _db = sql.postgres(
            host=config.get("host", "localhost"),
            port=config.get("port", 5432),
            user=config.get("user"),
            password=config.get("password"),
            dbname=config.get("database"),
            sslmode=config.get("sslmode", "disable"),
            max_open_conns=config.get("max_conns", 10),
        )
    elif db_type == "mysql":
        _db = sql.mysql(
            host=config.get("host", "localhost"),
            port=config.get("port", 3306),
            user=config.get("user"),
            password=config.get("password"),
            dbname=config.get("database"),
            max_open_conns=config.get("max_conns", 10),
        )
    elif db_type == "sqlite":
        _db = sql.sqlite(
            path=config.get("path", "/var/lib/data.db"),
        )
    elif db_type == "mssql":
        _db = sql.mssql(
            host=config.get("host", "localhost"),
            port=config.get("port", 1433),
            user=config.get("user"),
            password=config.get("password"),
            database=config.get("database"),
            max_open_conns=config.get("max_conns", 10),
        )
    else:
        return {"error": "Unsupported database type: " + db_type}
    
    # Test connection
    _db.ping()
    
    # Initialize schema if needed
    if config.get("auto_init", False):
        init_schema()
    
    print("Database connection initialized")
    return None

def validate(config):
    """Validate configuration."""
    required = ["db_type", "user", "password"]
    for field in required:
        if field not in config or not config[field]:
            return {"error": "%s is required" % field}
    return None

def init_schema():
    """Initialize database schema."""
    create_table = """
    CREATE TABLE IF NOT EXISTS users (
        id SERIAL PRIMARY KEY,
        email VARCHAR(255) UNIQUE NOT NULL,
        username VARCHAR(255) NOT NULL,
        display_name VARCHAR(255),
        disabled BOOLEAN DEFAULT FALSE,
        attributes JSONB,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    )
    """
    
    _db.exec(create_table)
    print("Schema initialized")

def sync(state):
    """Sync identities to database."""
    result = {
        "identities_created": 0,
        "identities_updated": 0,
        "identities_deleted": 0,
        "groups_created": 0,
        "groups_updated": 0,
        "groups_deleted": 0,
        "errors": [],
    }
    
    identities = state["identities"]
    dry_run = state["dry_run"]
    
    # Use transaction for atomicity
    if not dry_run:
        tx = _db.begin()
    
    try:
        for uid, identity in identities.items():
            email = identity["email"]
            if not email:
                continue
            
            if identity["disabled"]:
                # Delete or disable user
                if dry_run:
                    print("[DRY RUN] Would disable user:", email)
                    result["identities_deleted"] += 1
                else:
                    res = tx.exec(
                        "UPDATE users SET disabled = TRUE WHERE email = $1",
                        [email],
                    )
                    if res["rows_affected"] > 0:
                        result["identities_deleted"] += 1
                continue
            
            # Check if user exists
            existing = _db.query_row(
                "SELECT id, username, display_name FROM users WHERE email = $1",
                [email],
            )
            
            if existing == None:
                # Create new user
                if dry_run:
                    print("[DRY RUN] Would create user:", email)
                    result["identities_created"] += 1
                else:
                    attrs_json = json.encode(identity["attributes"])
                    tx.exec(
                        """
                        INSERT INTO users (email, username, display_name, attributes)
                        VALUES ($1, $2, $3, $4)
                        """,
                        [email, identity["username"], identity["display_name"], attrs_json],
                    )
                    result["identities_created"] += 1
                    print("Created user:", email)
            else:
                # Update existing user
                changes = []
                if existing["username"] != identity["username"]:
                    changes.append("username")
                if existing["display_name"] != identity["display_name"]:
                    changes.append("display_name")
                
                if changes:
                    if dry_run:
                        print("[DRY RUN] Would update user:", email, "fields:", changes)
                        result["identities_updated"] += 1
                    else:
                        attrs_json = json.encode(identity["attributes"])
                        tx.exec(
                            """
                            UPDATE users 
                            SET username = $1, 
                                display_name = $2, 
                                attributes = $3,
                                updated_at = CURRENT_TIMESTAMP
                            WHERE email = $4
                            """,
                            [identity["username"], identity["display_name"], attrs_json, email],
                        )
                        result["identities_updated"] += 1
                        print("Updated user:", email)
        
        # Commit transaction
        if not dry_run:
            tx.commit()
            print("Transaction committed")
    
    except Exception as e:
        # Rollback on error
        if not dry_run:
            tx.rollback()
        result["errors"].append(str(e))
        print("Error during sync, rolled back:", str(e))
    
    return result

def get_user_stats():
    """Get statistics about users in database."""
    stats = _db.query_row("""
        SELECT 
            COUNT(*) as total_users,
            COUNT(CASE WHEN disabled = TRUE THEN 1 END) as disabled_users,
            COUNT(CASE WHEN disabled = FALSE THEN 1 END) as active_users
        FROM users
    """)
    
    return stats

def search_users(search_term):
    """Search for users by email or username."""
    results = _db.query(
        """
        SELECT email, username, display_name, disabled
        FROM users
        WHERE email LIKE $1 OR username LIKE $1
        LIMIT 100
        """,
        ["%" + search_term + "%"],
    )
    
    return results

def close():
    """Close database connection."""
    _db.close()
    print("Database connection closed")
    return None
