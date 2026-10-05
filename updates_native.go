//go:build darwin && !server

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func nativeUpdatesSupported() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.HasSuffix(filepath.Dir(exe), "/Contents/MacOS") && strings.HasSuffix(filepath.Dir(filepath.Dir(filepath.Dir(exe))), ".app")
}

func checkUpdateLocation() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") {
		return fmt.Errorf("请使用打包后的 AI Atlas 应用")
	}
	// Probe the containing directory before quitting: read-only volumes and
	// app translocation must not strand the user after the old process exits.
	f, err := os.CreateTemp(filepath.Dir(bundle), ".ai-atlas-update-check-*")
	if err != nil {
		return fmt.Errorf("应用所在目录不可写，请将 AI Atlas 移到可写目录后再更新：%w", err)
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	return errors.Join(closeErr, removeErr)
}
