
package rtaudiowrapper

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	output  = "rtaudio_go.o"
	cppFile = "./lib/rtaudio_go.cpp"
)

type audioBackend struct {
	name      string
	pkgConfig string
	define    string
	libs      []string
}

var linuxBackends = []audioBackend{
	{"JACK", "jack", "__UNIX_JACK__", []string{"-ljack"}},
	{"PulseAudio", "libpulse", "__LINUX_PULSE__", []string{"-lpulse", "-lpulse-simple"}},
	{"ALSA", "alsa", "__LINUX_ALSA__", []string{"-lasound"}},
}


func build() error {
	switch runtime.GOOS {
	case "windows":
		return buildWindows()
	case "linux":
		return buildLinux()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func buildWindows() error {
	fmt.Println("Building for Windows with WASAPI...")

	// Generate CGO flags file for Windows (empty since flags are in record.go)
	if err := writeCGOFlags(audioBackend{name: "WASAPI", libs: []string{}}); err != nil {
		return fmt.Errorf("failed to write CGO flags: %w", err)
	}

	winLibs := []string{
		"-lole32",
		"-lwinmm",
		"-lksuser",
		"-lmfplat",
		"-lmfuuid",
		"-lwmcodecdspuuid",
	}

	args := []string{
		"-c",
		"-o", output,
		cppFile,
		"-D__WINDOWS_WASAPI__",
	}
	args = append(args, winLibs...)

	return runCommand("g++", args...)
}

func buildLinux() error {
	fmt.Println("Detecting available audio backends...")

	// Check which backends are available
	var available []audioBackend
	for _, backend := range linuxBackends {
		if hasBackend(backend.pkgConfig) {
			available = append(available, backend)
			fmt.Printf("  ✓ %s found\n", backend.name)
		}
	}

	if len(available) == 0 {
		return handleNoBackend()
	}

	// Use the first available backend
	selected := available[0]
	fmt.Printf("\nUsing %s for audio backend\n", selected.name)

	// Generate CGO flags file with backend-specific linking
	if err := writeCGOFlags(selected); err != nil {
		return fmt.Errorf("failed to write CGO flags: %w", err)
	}

	return buildLinuxWithBackend(selected.define, selected.libs)
}

func buildLinuxWithBackend(define string, libs []string) error {
	args := []string{
		"-c",
		"-o", output,
		cppFile,
		fmt.Sprintf("-D%s", define),
		"-lpthread",
		"-lm",
		"-lstdc++",
	}
	args = append(args, libs...)

	return runCommand("g++", args...)
}

func hasBackend(pkgName string) bool {
	cmd := exec.Command("pkg-config", "--exists", pkgName)
	return cmd.Run() == nil
}

func handleNoBackend() error {
	fmt.Println("\n❌ No audio backend found!")
	fmt.Println("\nYou need to install one of the following:")

	distro := detectDistro()

	switch distro {
	case "debian", "ubuntu":
		fmt.Println("\n  # Debian/Ubuntu:")
		fmt.Println("  sudo apt install libasound2-dev      # ALSA (stable)")
		fmt.Println("  sudo apt install libjack-jackd2-dev  # JACK2 (fastest)")
		fmt.Println("  sudo apt install libpulse-dev        # PulseAudio")
	case "fedora", "rhel", "centos":
		fmt.Println("\n  # Fedora/RHEL/CentOS:")
		fmt.Println("  sudo dnf install alsa-lib-devel          # ALSA (stable)")
		fmt.Println("  sudo dnf install jack-audio-connection-kit-devel  # JACK (fastest)")
		fmt.Println("  sudo dnf install pulseaudio-libs-devel   # PulseAudio")
	case "arch":
		fmt.Println("\n  # Arch Linux:")
		fmt.Println("  sudo pacman -S alsa-lib                  # ALSA (stable)")
		fmt.Println("  sudo pacman -S jack2                     # JACK2 (fastest)")
		fmt.Println("  sudo pacman -S libpulse                  # PulseAudio")
	default:
		fmt.Println("\n  Please install development packages for one of:")
		fmt.Println("    - ALSA (libasound/alsa-lib)")
		fmt.Println("    - PulseAudio (libpulse/pulseaudio-libs)")
		fmt.Println("    - JACK/JACK2 (libjack/jack-audio-connection-kit)")
	}

	fmt.Println("\nWould you like to install one now? (y/N)")
	if !askConfirmation() {
		return fmt.Errorf("audio backend required to build")
	}

	return installBackend(distro)
}

func detectDistro() string {
	// Check /etc/os-release
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}

	content := string(data)
	if strings.Contains(strings.ToLower(content), "ubuntu") {
		return "ubuntu"
	}
	if strings.Contains(strings.ToLower(content), "debian") {
		return "debian"
	}
	if strings.Contains(strings.ToLower(content), "fedora") {
		return "fedora"
	}
	if strings.Contains(strings.ToLower(content), "rhel") || strings.Contains(strings.ToLower(content), "red hat") {
		return "rhel"
	}
	if strings.Contains(strings.ToLower(content), "centos") {
		return "centos"
	}
	if strings.Contains(strings.ToLower(content), "arch") {
		return "arch"
	}

	return "unknown"
}

func askConfirmation() bool {
	// When running under go generate, stdin is not connected to the terminal.
	// We need to explicitly open /dev/tty to read from the terminal.
	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Println("\nCouldn't open the terminal input, try installing the dependency yourself with the previously mentioned command.")
		// If we can't open the terminal, default to no
		return false
	}
	defer tty.Close()

	reader := bufio.NewReader(tty)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// TODO: Allow for installing of backend based on user input choice
func installBackend(distro string) error {
	var cmd *exec.Cmd

	switch distro {
	case "debian", "ubuntu":
		fmt.Println("\nInstalling ALSA (recommended for best compatibility)...")
		fmt.Println("Running: sudo apt install -y libasound2-dev")
		fmt.Println("\nProceed? (y/N)")
		if !askConfirmation() {
			return fmt.Errorf("installation cancelled")
		}
		cmd = exec.Command("sudo", "apt", "install", "-y", "libasound2-dev")
	case "fedora", "rhel", "centos":
		fmt.Println("\nInstalling ALSA (recommended for best compatibility)...")
		fmt.Println("Running: sudo dnf install -y alsa-lib-devel")
		fmt.Println("\nProceed? (y/N)")
		if !askConfirmation() {
			return fmt.Errorf("installation cancelled")
		}
		cmd = exec.Command("sudo", "dnf", "install", "-y", "alsa-lib-devel")
	case "arch":
		fmt.Println("\nInstalling ALSA (recommended for best compatibility)...")
		fmt.Println("Running: sudo pacman -S --noconfirm alsa-lib")
		fmt.Println("\nProceed? (y/N)")
		if !askConfirmation() {
			return fmt.Errorf("installation cancelled")
		}
		cmd = exec.Command("sudo", "pacman", "-S", "--noconfirm", "alsa-lib")
	default:
		return fmt.Errorf("automatic installation not supported for your distribution")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Println("\n✓ Installation successful! Retrying build...")
	return buildLinux()
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Running: %s %v\n", name, args)

	return cmd.Run()
}

func writeCGOFlags(backend audioBackend) error {
	// For Linux backends, ensure library order:
	// 	object files first, then C++ stdlib, then backend-specific libs
	var ldflags string
	if len(backend.libs) > 0 {
		// Append backend libs after the common libs
		ldflags = fmt.Sprintf("${SRCDIR}/rtaudio_go.o -lstdc++ -lm %s -g", strings.Join(backend.libs, " "))
	} else {
		ldflags = "${SRCDIR}/rtaudio_go.o -lstdc++ -lm -g"
	}

	content := fmt.Sprintf(`// Code generated by build.go. DO NOT EDIT.

package rtaudiowrapper

/*
#cgo linux LDFLAGS: %s
*/
import "C"
`, ldflags)

	// Write to the same directory as the build script
	if err := os.WriteFile("cgo_flags.go", []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write cgo_flags.go: %w", err)
	}

	fmt.Printf("Generated cgo_flags.go with LDFLAGS: %s\n", ldflags)
	return nil
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	if err := build(); err != nil {
		fatal("Build failed: %v", err)
	}
	fmt.Println("Build successful!")
}
