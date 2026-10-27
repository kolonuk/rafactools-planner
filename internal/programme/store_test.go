package programme

import (
	"path/filepath"
	"testing"

	"rafactools-planner/internal/db"
)

// A category ID belonging to another squadron's programme must not be usable
// to add or delete entries via your own programme ID.
func TestRequirementEntriesAreScopedToProgramme(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, n := range []string{"1", "2"} {
		d.Exec(`INSERT INTO squadrons (number, name) VALUES (?, ?)`, n, n)
		d.Exec(`INSERT INTO users (squadron_id, email, password_hash, role) VALUES (?, ?, 'x', 'owner')`, n, n+"@x")
	}
	st := NewStore(d)
	mine, err := st.CreateProgramme(1, 2030, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := st.CreateProgramme(2, 2030, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	cat, err := st.AddRequirementCategory(theirs.ID, "Duty", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.AddRequirementEntry(mine.ID, cat, "Cdt", ""); err == nil {
		t.Fatal("add into another programme's category succeeded")
	}
	if err := st.AddRequirementEntry(theirs.ID, cat, "Cdt", ""); err != nil {
		t.Fatalf("legit add failed: %v", err)
	}
	var id int64
	d.QueryRow(`SELECT id FROM programme_requirement_entries`).Scan(&id)
	st.DeleteRequirementEntry(mine.ID, cat, id)
	var n int
	d.QueryRow(`SELECT COUNT(*) FROM programme_requirement_entries`).Scan(&n)
	if n != 1 {
		t.Fatal("entry deleted via a different programme's ID")
	}
}
