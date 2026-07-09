//go:build unix

package procinfo

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func writeComm(t *testing.T, root string, pid int, name string) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir proc pid dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatalf("write comm: %v", err)
	}
}

func TestResolve_returnsNameAndUsername_whenCommAndLookupSucceed(t *testing.T) {
	// Given
	root := t.TempDir()
	writeComm(t, root, 4242, "python3")
	restoreRoot := procRoot
	restoreLookup := lookupUserID
	t.Cleanup(func() { procRoot = restoreRoot; lookupUserID = restoreLookup })
	procRoot = root
	lookupUserID = func(uid string) (*user.User, error) {
		return &user.User{Uid: uid, Username: "alice"}, nil
	}

	// When
	name, usr := Resolve(4242)

	// Then
	if name != "python3" {
		t.Fatalf("expected name python3, got %q", name)
	}
	if usr != "alice" {
		t.Fatalf("expected user alice, got %q", usr)
	}
}

func TestResolve_fallsBackToNumericUID_whenLookupFails(t *testing.T) {
	// Given
	root := t.TempDir()
	writeComm(t, root, 55, "worker")
	restoreRoot := procRoot
	restoreLookup := lookupUserID
	t.Cleanup(func() { procRoot = restoreRoot; lookupUserID = restoreLookup })
	procRoot = root
	lookupUserID = func(string) (*user.User, error) {
		return nil, user.UnknownUserIdError(0)
	}

	// When
	name, usr := Resolve(55)

	// Then
	if name != "worker" {
		t.Fatalf("expected name worker, got %q", name)
	}
	if _, err := strconv.Atoi(usr); err != nil {
		t.Fatalf("expected numeric uid fallback, got %q", usr)
	}
}

func TestResolve_returnsEmptyUser_whenStatFails(t *testing.T) {
	// Given
	root := t.TempDir()
	writeComm(t, root, 88, "svc")
	restoreRoot := procRoot
	restoreStat := statUID
	t.Cleanup(func() { procRoot = restoreRoot; statUID = restoreStat })
	procRoot = root
	statUID = func(string) (uint32, bool) { return 0, false }

	// When
	name, usr := Resolve(88)

	// Then
	if name != "svc" {
		t.Fatalf("expected name svc, got %q", name)
	}
	if usr != "" {
		t.Fatalf("expected empty user when stat fails, got %q", usr)
	}
}

func TestResolve_returnsEmpty_whenPidDirMissing(t *testing.T) {
	// Given
	root := t.TempDir()
	restoreRoot := procRoot
	t.Cleanup(func() { procRoot = restoreRoot })
	procRoot = root

	// When
	name, usr := Resolve(999999)

	// Then
	if name != "" || usr != "" {
		t.Fatalf("expected empty name/user for missing pid, got %q/%q", name, usr)
	}
}

func TestResolve_returnsEmpty_whenCommMissingButDirExists(t *testing.T) {
	// Given
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "77"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	restoreRoot := procRoot
	t.Cleanup(func() { procRoot = restoreRoot })
	procRoot = root

	// When
	name, usr := Resolve(77)

	// Then
	if name != "" || usr != "" {
		t.Fatalf("expected empty name/user when comm missing, got %q/%q", name, usr)
	}
}
