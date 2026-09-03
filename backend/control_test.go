package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComposeProjectName(t *testing.T) {
	cases := map[string]string{
		"Matchmaker":       "matchmaker",
		"Scaleo":           "scaleo",
		"Secrets-storage":  "secrets-storage",
		"ops-console":      "ops-console",
		"Foo Bar":          "foobar",
		"-leading":         "p-leading",
		"_under":           "p_under",
		"":                 "stack",
		"Already_ok-1":     "already_ok-1",
	}
	for in, want := range cases {
		if got := composeProjectName(in); got != want {
			t.Fatalf("composeProjectName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSameStack(t *testing.T) {
	if !sameStack("Matchmaker", "matchmaker") {
		t.Fatal("Matchmaker should match matchmaker")
	}
	if sameStack("Matchmaker", "scaleo") {
		t.Fatal("Matchmaker must not match scaleo")
	}
}

func TestStackWantsMobile(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("DEPLOY_MOBILE=false\n")
	if stackWantsMobile(dir) {
		t.Fatal("false must not enable mobile")
	}
	write("DEPLOY_MOBILE=true\n")
	if !stackWantsMobile(dir) {
		t.Fatal("true must enable mobile")
	}
	write("# DEPLOY_MOBILE=true\nCOMPOSE_PROFILES=mobile\n")
	if !stackWantsMobile(dir) {
		t.Fatal("COMPOSE_PROFILES=mobile must enable mobile")
	}
	write("")
	if stackWantsMobile(dir) {
		t.Fatal("empty env must not enable mobile")
	}
}
