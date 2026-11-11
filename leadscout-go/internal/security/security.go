// Package security gates agent write/send/clear permissions and records a
// persistent agent audit log.
package security

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ali-ulu/lead/leadscout-go/internal/db"
)

// PermissionEnabled reports whether an env flag is set to "1".
func PermissionEnabled(name string, def bool) bool {
	if v, ok := os.LookupEnv(name); ok {
		return v == "1"
	}
	return def
}

// RequirePermission errors unless the named flag is enabled.
func RequirePermission(name, description string, def bool) error {
	if !PermissionEnabled(name, def) {
		return fmt.Errorf("%s is disabled. Set %s=1 to enable it.", description, name)
	}
	return nil
}

// AuditAgentAction records an action in the agent audit log.
func AuditAgentAction(action, target, status string, metadata map[string]any, actor string) error {
	conn, err := db.Conn()
	if err != nil {
		return err
	}
	defer conn.Close()
	if actor == "" {
		actor = "mcp"
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaJSON, _ := json.Marshal(metadata)
	var tgt any
	if target != "" {
		tgt = target
	}
	_, err = conn.Exec(
		"INSERT INTO agent_audit_log(actor,action,target,status,metadata) VALUES (?,?,?,?,?)",
		actor, action, tgt, status, string(metaJSON),
	)
	return err
}

// ListAgentAudit returns the most recent audit entries.
func ListAgentAudit(limit int) ([]map[string]any, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	conn, err := db.Conn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query("SELECT id,actor,action,target,status,metadata,created_at FROM agent_audit_log ORDER BY created_at DESC,id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var actor, action, status, metadata, createdAt string
		var target *string
		if err := rows.Scan(&id, &actor, &action, &target, &status, &metadata, &createdAt); err != nil {
			return nil, err
		}
		meta := map[string]any{}
		_ = json.Unmarshal([]byte(metadata), &meta)
		out = append(out, map[string]any{
			"id": id, "actor": actor, "action": action, "target": target,
			"status": status, "metadata": meta, "created_at": createdAt,
		})
	}
	return out, rows.Err()
}
