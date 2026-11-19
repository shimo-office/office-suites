package main

import (
	"debug/macho"
	"os/exec"
	"runtime"
)

// A universal Chrome launched directly can inherit translated execution on an
// Apple Silicon host. Select its native slice only after inspecting the binary.
func phase3ChromeCommand(chrome string, args ...string) *exec.Cmd {
	command, launchArgs := phase3ChromeLaunchSpec(runtime.GOOS, runtime.GOARCH, chrome, args, phase3ChromeHasARM64)
	return exec.Command(command, launchArgs...)
}

func phase3ChromeLaunchSpec(hostOS, hostArch, chrome string, args []string, hasARM64 func(string) bool) (string, []string) {
	if hostOS == "darwin" && hostArch == "arm64" && hasARM64(chrome) {
		return "/usr/bin/arch", append([]string{"-arm64", chrome}, args...)
	}
	return chrome, append([]string{}, args...)
}

func phase3ChromeHasARM64(filename string) bool {
	fat, err := macho.OpenFat(filename)
	if err == nil {
		defer fat.Close()
		for _, arch := range fat.Arches {
			if arch.Cpu == macho.CpuArm64 {
				return true
			}
		}
		return false
	}
	// A valid thin native executable also needs an explicit native launch. Other
	// errors fail conservatively; malformed or unreadable binaries get no override.
	if err != macho.ErrNotFat {
		return false
	}
	thin, err := macho.Open(filename)
	if err != nil {
		return false
	}
	defer thin.Close()
	return thin.Cpu == macho.CpuArm64
}
