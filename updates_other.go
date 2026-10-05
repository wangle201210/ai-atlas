//go:build !darwin || server

package main

import "fmt"

func nativeUpdatesSupported() bool { return false }

func checkUpdateLocation() error { return fmt.Errorf("当前运行方式不支持自动安装") }
