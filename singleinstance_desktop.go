//go:build !server

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func instanceOptions(key string, activate func()) *application.SingleInstanceOptions {
	return &application.SingleInstanceOptions{UniqueID: key, OnSecondInstanceLaunch: func(application.SecondInstanceData) { activate() }}
}
