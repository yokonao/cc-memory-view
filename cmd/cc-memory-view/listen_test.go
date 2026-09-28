package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenInvalidUnixAddress(t *testing.T) {
	for _, addr := range []string{"unix://host/x.sock", "unix:relative.sock", "unix:///x.sock?q=1"} {
		if ln, err := listen(addr); err == nil {
			_ = ln.Close()
			t.Errorf("listen(%q) succeeded", addr)
		}
	}
}

func TestListenUnix(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s.sock")

	// A socket left behind by a dead process is replaced.
	stale, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = stale.Close()

	ln, err := listen("unix://" + sock)
	if err != nil {
		t.Fatalf("stale socket: %v", err)
	}

	if other, err := listen("unix://" + sock); err == nil {
		_ = other.Close()
		t.Error("listened on a socket in use")
	}

	_ = ln.Close()
	if _, err := os.Lstat(sock); !os.IsNotExist(err) {
		t.Errorf("socket not removed on close: %v", err)
	}
}

func TestListenUnixNotSocket(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if ln, err := listen("unix://" + file); err == nil {
		_ = ln.Close()
		t.Error("replaced a regular file")
	}
}
