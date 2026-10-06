//go:build darwin

package atlas

import "golang.org/x/sys/unix"

func renameExclusive(from, to string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_EXCL)
}
