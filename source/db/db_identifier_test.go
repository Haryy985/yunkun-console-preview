package db

import "testing"

func TestCreateDatabaseSQLEscapesIdentifier(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"yunkun", `CREATE DATABASE "yunkun"`},
		{`name"; DROP DATABASE yunkun; --`, `CREATE DATABASE "name""; DROP DATABASE yunkun; --"`},
	} {
		if got := createDatabaseSQL(tc.name); got != tc.want {
			t.Errorf("createDatabaseSQL(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
