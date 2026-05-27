package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"k3slab/kube"
)

const terminalBashRC = "/usr/local/lib/k3slab/terminal-bashrc.sh"

// terminalStartDir is the shell's initial working directory (not LAB_ROOT).
// Override with K3SLAB_TERMINAL_CWD if needed.
func terminalStartDir() string {
	if d := strings.TrimSpace(os.Getenv("K3SLAB_TERMINAL_CWD")); d != "" {
		return d
	}
	return "/root"
}

var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func terminalEnv() []string {
	return kube.Env("TERM=xterm-256color")
}

func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("terminal upgrade: %v", err)
		return
	}
	defer conn.Close()

	// --rcfile loads the k3slab prompt on the first line (bash -i alone may skip ~/.bashrc in a PTY).
	cmd := exec.Command("bash", "--rcfile", terminalBashRC, "-i")
	// Do not start in the lab tree so a casual `ls` does not expose workshop scripts.
	cmd.Dir = terminalStartDir()
	cmd.Env = terminalEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("pty start: %v", err)
		return
	}
	defer func() { _ = ptmx.Close() }()

	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		mt, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch mt {
		case websocket.BinaryMessage:
			if _, err := ptmx.Write(payload); err != nil {
				return
			}
		case websocket.TextMessage:
			var rm resizeMsg
			if json.Unmarshal(payload, &rm) == nil && rm.Type == "resize" && rm.Cols > 0 && rm.Rows > 0 {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: rm.Rows, Cols: rm.Cols})
			}
		default:
			// ignore
		}
	}

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_, _ = cmd.Process.Wait()
}
