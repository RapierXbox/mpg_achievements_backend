package network

import (
	"runtime"

	"golang.org/x/sys/unix"
)

// syscall numbers for sendmmsg (architecture specific)
const (
	SYS_SENDMMSG_AMD64 = 307
	SYS_SENDMMSG_ARM64 = 269
	SYS_SENDMMSG_386   = 345
)

// GetSendmmsgSyscall returns the correct syscall number for the current architecture
func GetSendmmsgSyscall() uintptr {
	// try to use the syscall packages constant if available
	// otherwise fall back to architecture specific values
	switch runtime.GOARCH {
	case "amd64":
		return SYS_SENDMMSG_AMD64
	case "arm64":
		return SYS_SENDMMSG_ARM64
	case "386":
		return SYS_SENDMMSG_386
	default:
		return SYS_SENDMMSG_AMD64 // default to amd64
	}
}

// Mmsghdr is the message header for sendmmsg/recvmmsg
type Mmsghdr struct {
	Msghdr unix.Msghdr
	Msglen uint32
	_      [4]byte // padding for alignment
}
