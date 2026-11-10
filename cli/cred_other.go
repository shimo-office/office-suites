//go:build !darwin && !windows

package main

func keychainAvailable() bool           { return false }
func keychainSave(string, string) error { return nil }
func keychainLoad() string              { return "" }
func keychainClear()                    {}
