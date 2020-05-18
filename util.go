package main

import (
	"unsafe"
)

func s2b(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}
