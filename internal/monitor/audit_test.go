package monitor

import "testing"

func TestParseAuditUsersFiltersMalformedAndLimits(t *testing.T) {
	users, truncated := parseAuditUsers("root:x:0:0:root:/root:/bin/bash\nalice:x:1000:1000::/home/alice:/bin/sh\nbroken\n", 1)
	if !truncated || len(users) != 1 || users[0].Name != "alice" || users[0].UID != 1000 {
		t.Fatalf("users = %#v, truncated = %v", users, truncated)
	}
}

func TestParseAuditEntriesSortsAndLimits(t *testing.T) {
	entries, truncated := parseAuditEntries("zlib\nacl\nbusybox\n", 2)
	if !truncated || len(entries) != 2 || entries[0] != "acl" || entries[1] != "busybox" {
		t.Fatalf("entries = %#v, truncated = %v", entries, truncated)
	}
}
