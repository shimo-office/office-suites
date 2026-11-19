package main

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhase3ChromeLaunchArchitectureAndArgumentPreservation(t *testing.T) {
	chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	original := []string{"--no-first-run", "--user-data-dir=/private/tmp/profile with spaces", "--remote-debugging-port=0", "about:blank"}
	for _, scenario := range []struct {
		os, arch   string
		armSlice   bool
		wantNative bool
	}{
		{"darwin", "arm64", true, true}, {"darwin", "arm64", false, false},
		{"darwin", "amd64", true, false}, {"linux", "arm64", true, false}, {"windows", "arm64", true, false},
	} {
		t.Run(scenario.os+"/"+scenario.arch+"/"+map[bool]string{true: "arm-slice", false: "intel-only"}[scenario.armSlice], func(t *testing.T) {
			inspections := 0
			command, args := phase3ChromeLaunchSpec(scenario.os, scenario.arch, chrome, original, func(path string) bool {
				inspections++
				if path != chrome {
					t.Fatal(path)
				}
				return scenario.armSlice
			})
			expectedCommand, expectedArgs := chrome, append([]string{}, original...)
			if scenario.wantNative {
				expectedCommand = "/usr/bin/arch"
				expectedArgs = append([]string{"-arm64", chrome}, original...)
			}
			if command != expectedCommand || !reflect.DeepEqual(args, expectedArgs) {
				t.Fatalf("command=%q args=%q", command, args)
			}
			if (scenario.os != "darwin" || scenario.arch != "arm64") && inspections != 0 {
				t.Fatal("unnecessary platform inspection")
			}
			args[len(args)-1] = "modified output"
			if original[len(original)-1] != "about:blank" {
				t.Fatal("launch mutated original args")
			}
		})
	}
}

func TestPhase3ChromeMachOArchitectureDetection(t *testing.T) {
	thin := func(cpu macho.Cpu) []byte {
		buf := new(bytes.Buffer)
		for _, word := range []uint32{macho.Magic64, uint32(cpu), 0, 2, 0, 0, 0, 0} {
			_ = binary.Write(buf, binary.LittleEndian, word)
		}
		return buf.Bytes()
	}
	fat := func(cpus ...macho.Cpu) []byte {
		buf := new(bytes.Buffer)
		_ = binary.Write(buf, binary.BigEndian, uint32(macho.MagicFat))
		_ = binary.Write(buf, binary.BigEndian, uint32(len(cpus)))
		offset := uint32(8 + 20*len(cpus))
		for _, cpu := range cpus {
			for _, word := range []uint32{uint32(cpu), 0, offset, 32, 0} {
				_ = binary.Write(buf, binary.BigEndian, word)
			}
			offset += 32
		}
		for _, cpu := range cpus {
			buf.Write(thin(cpu))
		}
		return buf.Bytes()
	}
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{
		{"thin-arm", thin(macho.CpuArm64), true}, {"thin-intel", thin(macho.CpuAmd64), false},
		{"universal", fat(macho.CpuAmd64, macho.CpuArm64), true}, {"fat-intel", fat(macho.CpuAmd64), false},
		{"invalid", []byte("not a Mach-O"), false}, {"truncated", fat(macho.CpuAmd64, macho.CpuArm64)[:35], false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Chrome")
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			if got := phase3ChromeHasARM64(path); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if phase3ChromeHasARM64(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("missing executable treated as native")
	}
}
