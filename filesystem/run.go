package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/lastnameswayne/tinycontainer/db"
)

// rootfsPath is the absolute path to the FUSE mount's app directory, used as
// the runc container rootfs. Set once at startup from the CLI mount argument.
var rootfsPath string

var ansiRegex = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

type RunRequest struct {
	FileName string
	Username string
}

var runcConfigTemplateStr = `{
    "ociVersion": "1.2.0",
    "process": {
        "terminal": false,
        "user": {
            "uid": 0,
            "gid": 0
        },
        "args": [
            "/usr/bin/env",
            "python3",
            "/%s"
        ],
        "env": [
            "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "TERM=xterm"
        ],
        "cwd": "/",
        "capabilities": {
            "bounding": [
                "CAP_AUDIT_WRITE",
                "CAP_KILL",
                "CAP_NET_BIND_SERVICE"
            ],
            "effective": [
                "CAP_AUDIT_WRITE",
                "CAP_KILL",
                "CAP_NET_BIND_SERVICE"
            ],
            "permitted": [
                "CAP_AUDIT_WRITE",
                "CAP_KILL",
                "CAP_NET_BIND_SERVICE"
            ]
        },
        "rlimits": [
            {
                "type": "RLIMIT_NOFILE",
                "hard": 1024,
                "soft": 1024
            }
        ],
        "noNewPrivileges": true
    },
    "root": {
        "path": "%s",
        "readonly": false
    },
    "hostname": "runc",
    "mounts": [
        {
            "destination": "/proc",
            "type": "proc",
            "source": "proc"
        },
        {
            "destination": "/lib64",
            "type": "bind",
            "source": "%s",
            "options": ["rbind", "ro"]
        },
        {
            "destination": "/dev",
            "type": "tmpfs",
            "source": "tmpfs",
            "options": [
                "nosuid",
                "strictatime",
                "mode=755",
                "size=65536k"
            ]
        },
        {
            "destination": "/dev/pts",
            "type": "devpts",
            "source": "devpts",
            "options": [
                "nosuid",
                "noexec",
                "newinstance",
                "ptmxmode=0666",
                "mode=0620",
                "gid=5"
            ]
        },
        {
            "destination": "/dev/shm",
            "type": "tmpfs",
            "source": "shm",
            "options": [
                "nosuid",
                "noexec",
                "nodev",
                "mode=1777",
                "size=65536k"
            ]
        },
        {
            "destination": "/dev/mqueue",
            "type": "mqueue",
            "source": "mqueue",
            "options": [
                "nosuid",
                "noexec",
                "nodev"
            ]
        },
        {
            "destination": "/sys",
            "type": "sysfs",
            "source": "sysfs",
            "options": [
                "nosuid",
                "noexec",
                "nodev",
                "ro"
            ]
        },
        {
            "destination": "/sys/fs/cgroup",
            "type": "cgroup",
            "source": "cgroup",
            "options": [
                "nosuid",
                "noexec",
                "nodev",
                "relatime",
                "ro"
            ]
        }
    ],
    "linux": {
        "resources": {
            "memory": {
                "limit": 1073741824,
                "swap": 1073741824
            },
            "cpu": {
                "quota": 100000,
                "period": 100000
            },
            "pids": {
                "limit": 128
            },
            "devices": [
                {
                    "allow": false,
                    "access": "rwm"
                }
            ]
        },
        "namespaces": [
            {"type": "pid"},
            {"type": "network"},
            {"type": "ipc"},
            {"type": "uts"},
            {"type": "mount"},
            {"type": "cgroup"}
        ],
        "maskedPaths": [
            "/proc/acpi",
            "/proc/asound",
            "/proc/kcore",
            "/proc/keys",
            "/proc/latency_stats",
            "/proc/timer_list",
            "/proc/timer_stats",
            "/proc/sched_debug",
            "/sys/firmware",
            "/proc/scsi"
        ],
        "readonlyPaths": [
            "/proc/bus",
            "/proc/fs",
            "/proc/irq",
            "/proc/sys",
            "/proc/sysrq-trigger"
        ]
    }
}
`

const _runcTimeout = 30 * time.Minute

type RunResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
	RunId    int    `json:"run_id"`
}

func (fs *FS) Run(w http.ResponseWriter, r *http.Request) {
	req := RunRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	fileName := req.FileName
	if fileName == "" {
		http.Error(w, "filename is required", http.StatusBadRequest)
		return
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_.\-]+$`, fileName)
	if !matched {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	fs.ClearNotFound()

	// create a per-run bundle directory so concurrent runs don't share config.json
	bundleDir, err := os.MkdirTemp("", "runc-bundle-*")
	if err != nil {
		http.Error(w, "Failed to create bundle dir: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(bundleDir)

	runcConfig := fmt.Sprintf(runcConfigTemplateStr, fileName, rootfsPath, filepath.Join(rootfsPath, "usr", "lib64"))
	if err := os.WriteFile(filepath.Join(bundleDir, "config.json"), []byte(runcConfig), 0644); err != nil {
		http.Error(w, "Failed to write config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// run runc command
	startTime := time.Now()
	containerID := fmt.Sprintf("container-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), _runcTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sudo", "runc", "run", "--bundle", bundleDir, containerID)

	stdout, err := cmd.Output()

	exec.Command("sudo", "runc", "delete", containerID).Run()
	duration := time.Since(startTime)
	exitCode := 0
	stderrStr := ""

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			stderrStr = string(exitErr.Stderr)
		} else {
			http.Error(w, "Failed to run container: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	memoryHits, diskHits, serverFetches := getAndResetLookupStats()

	username := req.Username

	id := int64(0)
	if db.DB != nil {
		id, err = db.LogRun(fileName, startTime, duration.Milliseconds(),
			stripANSI(string(stdout)), stderrStr, exitCode,
			memoryHits, diskHits, serverFetches, username)
		if err != nil {
			fmt.Println("Error logging run to database:", err)
		}
	}

	// write stdout back to user
	response := RunResponse{
		RunId:    int(id),
		Stdout:   string(stdout),
		Stderr:   stderrStr,
		ExitCode: exitCode,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func Stats(w http.ResponseWriter, r *http.Request) {
	runs, err := db.GetAllRuns()
	if err != nil {
		http.Error(w, "Failed to get runs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

func getAndResetLookupStats() (memoryHits, diskHits, serverFetches int64) {
	memoryHits = LookupStats.MemoryCacheHits.Swap(0)
	diskHits = LookupStats.DiskCacheHits.Swap(0)
	serverFetches = LookupStats.ServerFetches.Swap(0)
	return
}
