//go:build !darwin

package atlas

import "errors"

func renameExclusive(from, to string) error {
	return errors.New("临时文件移入废纸篓与恢复目前仅支持 macOS")
}
