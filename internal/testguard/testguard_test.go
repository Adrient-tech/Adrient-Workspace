package testguard

import "testing"

func TestDBName(t *testing.T) {
	for dsn, want := range map[string]string{
		"postgres://adrient@127.0.0.1:5533/adrient?sslmode=disable": "adrient",
		"postgres://a@h/w6fin_test":                                 "w6fin_test",
		"host=h dbname=zitadel user=u":                              "zitadel",
	} {
		if got := DBName(dsn); got != want {
			t.Fatalf("%s: %q", dsn, got)
		}
	}
	if !Protected["adrient"] || !Protected["zitadel"] || Protected["w6fin_test"] {
		t.Fatal("protected set")
	}
}
