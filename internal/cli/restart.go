package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// RestartSelf restarts the g3a process.
// If running under a supervisor/parent (like AClaw/supervise-daemon), it signals/terminates
// the existing instance so the supervisor respawns the latest binary.
// If standalone, it launches a new instance before exiting.
func RestartSelf(args []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("eval executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("eval symlink: %w", err)
	}

	myPID := os.Getpid()

	if runtime.GOOS == "windows" {
		return restartWindows(exePath, myPID, args)
	}
	return restartUnix(exePath, myPID, args)
}

func restartUnix(exePath string, myPID int, args []string) error {
	// Find any other running 'g3a mcp' processes
	pids := findUnixG3APIDs(myPID)
	for _, pid := range pids {
		proc, err := os.FindProcess(pid)
		if err == nil {
			fmt.Printf("Menghentikan instance lama PID %d...\n", pid)
			_ = proc.Signal(os.Interrupt)
			time.Sleep(300 * time.Millisecond)
			_ = proc.Kill()
		}
	}

	// If args was empty, default to "mcp"
	if len(args) == 0 {
		args = []string{"mcp"}
	}

	// Spawn new process
	cmd := exec.Command(exePath, args...)
	cmd.Dir, _ = os.Getwd()
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new g3a process: %w", err)
	}

	fmt.Printf("✅ g3a berhasil direstart! PID baru: %d\n", cmd.Process.Pid)
	return nil
}

func restartWindows(exePath string, myPID int, args []string) error {
	// On Windows, kill existing g3a.exe processes except self
	_ = exec.Command("taskkill", "/F", "/FI", fmt.Sprintf("PID ne %d", myPID), "/IM", "g3a.exe").Run()

	if len(args) == 0 {
		args = []string{"mcp"}
	}

	cmd := exec.Command(exePath, args...)
	cmd.Dir, _ = os.Getwd()
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new g3a.exe process: %w", err)
	}

	fmt.Printf("✅ g3a berhasil direstart! PID baru: %d\n", cmd.Process.Pid)
	return nil
}

func findUnixG3APIDs(excludePID int) []int {
	out, err := exec.Command("pgrep", "-f", "g3a mcp").Output()
	if err != nil {
		return nil
	}

	var pids []int
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if p, err := strconv.Atoi(line); err == nil && p != excludePID {
			pids = append(pids, p)
		}
	}
	return pids
}
