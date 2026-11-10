//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

var (
	advapi32        = syscall.NewLazyDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type filetime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

type winCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func keychainAvailable() bool { return true }

func keychainSave(account, cookie string) error {
	target, err := syscall.UTF16PtrFromString(keychainService)
	if err != nil {
		return err
	}
	user, err := syscall.UTF16PtrFromString(account)
	if err != nil {
		return err
	}
	blob := []byte(cookie)
	var blobPtr *byte
	if len(blob) > 0 {
		blobPtr = &blob[0]
	}
	c := winCredential{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     blobPtr,
		Persist:            credPersistLocalMachine,
		UserName:           user,
	}
	r, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0)
	if r == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return errors.New("CredWriteW failed")
	}
	return nil
}

func keychainLoad() string {
	target, err := syscall.UTF16PtrFromString(keychainService)
	if err != nil {
		return ""
	}
	var p *winCredential
	r, _, _ := procCredReadW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 || p == nil {
		return ""
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(p)))
	if p.CredentialBlobSize == 0 || p.CredentialBlob == nil {
		return ""
	}
	b := unsafe.Slice(p.CredentialBlob, p.CredentialBlobSize)
	return string(b)
}

func keychainClear() {
	target, err := syscall.UTF16PtrFromString(keychainService)
	if err != nil {
		return
	}
	_, _, _ = procCredDeleteW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
}
