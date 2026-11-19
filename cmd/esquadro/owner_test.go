package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestAPIOfRecordsOwnedInTheSameModule generates records owned by a
// principal of their own module (a key to its table) and runs their API's
// generated tests, which sign in as one owning every record.
func TestAPIOfRecordsOwnedInTheSameModule(t *testing.T) {
	if testing.Short() {
		t.Skip("generates the api starter, which needs sqlc or Docker")
	}
	dir := filepath.Join(t.TempDir(), "shop")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/shop", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	accounts := filepath.Join(dir, "esquadro", "accounts.esq")
	os.WriteFile(accounts, []byte(read(t, accounts)+"\n/// Where a customer receives orders.\nentity Address {\n  street   string required\n  customer > Customer required\n  @security(read owner, write owner)\n}\n"), 0o644)
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	cmd := exec.Command("go", "test", "./cmd/shop/", "-run", "TestAPIAddresses|TestAPIAccess")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the API's tests of addresses: %v\n%s", err, out)
	}
}
