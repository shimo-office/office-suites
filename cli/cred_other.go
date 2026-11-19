//go:build !darwin && !windows

package main

func keychainAvailable() bool                   { return false }
func keychainSave(string, string, string) error { return nil }
func keychainLoad(service string) string        { return "" }
func keychainClear(service string)              {}
