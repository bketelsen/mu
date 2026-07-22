package userdb

import "testing"

func ids(rs []Record) map[string]bool {
	m := map[string]bool{}
	for _, r := range rs {
		m[r.ID] = true
	}
	return m
}

func TestFilterAppliesWhere(t *testing.T) {
	recs := []Record{
		{ID: "1", Data: map[string]interface{}{"title": "keep"}},
		{ID: "2", Data: map[string]interface{}{"title": "drop"}},
	}
	got := ids(filter(recs, map[string]interface{}{"title": "keep"}))
	if !got["1"] || got["2"] {
		t.Fatalf("where filter wrong: %v", got)
	}
}

func TestFilterNoWhereReturnsAll(t *testing.T) {
	recs := []Record{
		{ID: "1", Data: map[string]interface{}{"title": "a"}},
		{ID: "2", Data: map[string]interface{}{"title": "b"}},
	}
	if got := filter(recs, nil); len(got) != 2 {
		t.Fatalf("filter(nil) returned %d records, want 2", len(got))
	}
}

func TestWhereOperators(t *testing.T) {
	rec := Record{Data: map[string]interface{}{
		"age": float64(30), "title": "My Note",
		"tags": []interface{}{"work", "ideas"}, "done": false,
	}}
	cases := []struct {
		name  string
		where map[string]interface{}
		want  bool
	}{
		{"eq scalar", map[string]interface{}{"age": float64(30)}, true},
		{"gte", map[string]interface{}{"age": map[string]interface{}{"gte": float64(30)}}, true},
		{"gt fail", map[string]interface{}{"age": map[string]interface{}{"gt": float64(30)}}, false},
		{"ne true", map[string]interface{}{"done": map[string]interface{}{"ne": true}}, true},
		{"contains substr", map[string]interface{}{"title": map[string]interface{}{"contains": "note"}}, true},
		{"contains array", map[string]interface{}{"tags": map[string]interface{}{"contains": "ideas"}}, true},
		{"in", map[string]interface{}{"age": map[string]interface{}{"in": []interface{}{float64(20), float64(30)}}}, true},
		{"exists false miss", map[string]interface{}{"title": map[string]interface{}{"exists": false}}, false},
		{"combined", map[string]interface{}{"age": map[string]interface{}{"gte": float64(18), "lt": float64(65)}}, true},
	}
	for _, c := range cases {
		if got := matchesWhere(rec, c.where); got != c.want {
			t.Errorf("%s: matchesWhere = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSortRecordsByField(t *testing.T) {
	rs := []Record{
		{ID: "1", Data: map[string]interface{}{"n": float64(3)}},
		{ID: "2", Data: map[string]interface{}{"n": float64(1)}},
		{ID: "3", Data: map[string]interface{}{"n": float64(2)}},
	}
	sortRecords(rs, "n", "asc")
	if rs[0].ID != "2" || rs[2].ID != "1" {
		t.Fatalf("asc sort wrong: %v", ids(rs))
	}
}

// TestGuestsRejected ensures every operation requires an authenticated caller.
func TestGuestsRejected(t *testing.T) {
	if _, err := Create("api", "", "notes", map[string]interface{}{"a": "b"}); err != ErrAuth {
		t.Errorf("Create as guest = %v, want ErrAuth", err)
	}
	if _, err := Get("api", "", "notes", "id"); err != ErrAuth {
		t.Errorf("Get as guest = %v, want ErrAuth", err)
	}
	if _, err := List("api", "", "notes", nil, "", "", 0); err != ErrAuth {
		t.Errorf("List as guest = %v, want ErrAuth", err)
	}
	if _, err := Update("api", "", "notes", "id", nil); err != ErrAuth {
		t.Errorf("Update as guest = %v, want ErrAuth", err)
	}
	if err := Delete("api", "", "notes", "id"); err != ErrAuth {
		t.Errorf("Delete as guest = %v, want ErrAuth", err)
	}
}

// TestKeyRejectsTraversal ensures namespace/collection can't escape the store.
func TestKeyRejectsTraversal(t *testing.T) {
	bad := [][2]string{
		{"apps/../etc", "notes"},
		{"api", "../secret"},
		{"api", "a/b"},
		{"API", "notes"}, // uppercase not allowed
	}
	for _, c := range bad {
		if _, err := key(c[0], c[1]); err == nil {
			t.Errorf("key(%q,%q) allowed, want rejection", c[0], c[1])
		}
	}
	if _, err := key("apps/notes-app", "my_notes"); err != nil {
		t.Errorf("valid key rejected: %v", err)
	}
}
