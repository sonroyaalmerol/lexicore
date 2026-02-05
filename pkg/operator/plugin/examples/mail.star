name = "mail"

def initialize(config):
    """Initialize with HTTP client and LDAP connection."""
    url = config.get("url")
    username = config.get("username")
    password = config.get("password")
    
    if not url or not username or not password:
        return {"error": "url, username, and password are required"}
    
    # Store config globally
    global _config
    _config = config
    
    # Create HTTP client with custom timeout
    global _http_client
    _http_client = http.client(timeout=60, insecure_skip_verify=False)
    
    # Login to get session
    login_data = {
        "username": username,
        "password": password,
    }
    
    resp = http.post(
        url + "/api/login",
        data=json.encode(login_data),
        headers={"Content-Type": "application/json"},
    )
    
    if not resp["ok"]:
        return {"error": "Login failed: " + resp["body"]}
    
    print("Successfully initialized mail operator")
    return None

def validate(config):
    """Validate configuration."""
    required = ["url", "username", "password"]
    for field in required:
        if field not in config or not config[field]:
            return {"error": "%s is required" % field}
    return None

def sync(state):
    """Sync identities using HTTP and LDAP."""
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
    
    # Optional: Connect to LDAP for additional queries
    if _config.get("ldap_url"):
        ldap_conn = ldap.connect(
            _config["ldap_url"],
            use_tls=True,
        )
        ldap_conn.bind(
            _config["ldap_bind_dn"],
            _config["ldap_bind_password"],
        )
    
    for uid, identity in identities.items():
        email = identity["email"]
        if not email or identity["disabled"]:
            continue
        
        # Check if user exists via HTTP
        resp = http.get(
            _config["url"] + "/api/user/" + email,
            headers={"Accept": "application/json"},
        )
        
        if resp["status_code"] == 404:
            # User doesn't exist, create
            if dry_run:
                print("[DRY RUN] Would create user:", email)
                result["identities_created"] += 1
            else:
                err = create_user(identity)
                if err:
                    result["errors"].append(err)
                else:
                    result["identities_created"] += 1
        elif resp["ok"]:
            # User exists, check for updates
            user_data = json.decode(resp["body"])
            changes = detect_changes(identity, user_data)
            
            if changes:
                if dry_run:
                    print("[DRY RUN] Would update:", email, changes)
                    result["identities_updated"] += 1
                else:
                    err = update_user(identity, changes)
                    if err:
                        result["errors"].append(err)
                    else:
                        result["identities_updated"] += 1
        else:
            result["errors"].append("Failed to check user %s: %s" % (email, resp["body"]))
    
    # Close LDAP connection if opened
    if _config.get("ldap_url"):
        ldap_conn.close()
    
    return result

def create_user(identity):
    """Create a new user via HTTP API."""
    email = identity["email"]
    
    user_data = {
        "cn": identity["display_name"] or identity["username"],
        "password": generate_password(),
        "mailQuota": identity["attributes"].get("mailQuota", "1024"),
    }
    
    # Add custom attributes
    for key, value in identity["attributes"].items():
        if key.startswith("mail:"):
            user_data[key[5:]] = value
    
    resp = http.post(
        _config["url"] + "/api/user/" + email,
        data=json.encode(user_data),
        headers={"Content-Type": "application/json"},
    )
    
    if not resp["ok"]:
        return "Failed to create %s: %s" % (email, resp["body"])
    
    print("Created user:", email)
    return None

def update_user(identity, changes):
    """Update user via HTTP API."""
    email = identity["email"]
    
    resp = http.put(
        _config["url"] + "/api/user/" + email,
        data=json.encode(changes),
        headers={"Content-Type": "application/json"},
    )
    
    if not resp["ok"]:
        return "Failed to update %s: %s" % (email, resp["body"])
    
    print("Updated user:", email)
    return None

def detect_changes(identity, user_data):
    """Detect what needs to be updated."""
    changes = {}
    
    expected_cn = identity["display_name"] or identity["username"]
    if user_data.get("cn") != expected_cn:
        changes["cn"] = expected_cn
    
    for key, value in identity["attributes"].items():
        if key.startswith("mail:"):
            field = key[5:]
            if user_data.get(field) != value:
                changes[field] = value
    
    return changes

def generate_password():
    """Generate a random password."""
    import time
    timestamp = time.unix()
    return hash.sha256(str(timestamp))[:16]

def close():
    """Cleanup resources."""
    print("Closing mail operator")
    return None
