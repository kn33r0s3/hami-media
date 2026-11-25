//go:build linux

// Command hami-media-launch is the systemd launcher for the hami-media
// server (job 17.2). It must NOT be a shell script: both dash and bash
// close file descriptors above 2 on startup, which would destroy the
// socket-activated listener systemd passes on fd 3. This Go program
// preserves all inherited fds, opens the bearer token file as fd 4
// (the binary reads its token there, never on argv — argv is visible
// via ps), and execs the server. The HMAC key comes from
// HAMI_SFU_HMAC_KEY_FILE in the environment.
package main

import (
	"fmt"
	"os"
	"syscall"
)

func main() {
	tokenFile := os.Getenv("HAMI_MEDIA_TOKEN_FILE")
	if tokenFile == "" {
		tokenFile = "/etc/hami-media/token.key"
	}
	f, err := os.Open(tokenFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hami-media-launch: open token file: %v\n", err)
		os.Exit(1)
	}
	info, err := f.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hami-media-launch: stat token file: %v\n", err)
		os.Exit(1)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "hami-media-launch: token file %s must be a regular file not readable by group or others (mode %04o)\n", tokenFile, uint32(info.Mode().Perm()))
		os.Exit(1)
	}
	// Duplicate the token file onto fd 4, the binary's token fd.
	if err := syscall.Dup3(int(f.Fd()), 4, 0); err != nil {
		fmt.Fprintf(os.Stderr, "hami-media-launch: dup token onto fd 4: %v\n", err)
		os.Exit(1)
	}
	if int(f.Fd()) != 4 {
		_ = f.Close()
	}
	binary := "/usr/local/bin/hami-media"
	if v := os.Getenv("HAMI_MEDIA_BINARY"); v != "" {
		binary = v
	}
	if err := syscall.Exec(binary, []string{binary, "-dev", "-mode", "m7h-sfu"}, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "hami-media-launch: exec %s: %v\n", binary, err)
		os.Exit(1)
	}
}
