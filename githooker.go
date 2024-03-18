package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ListenPort = os.Getenv("GH_LISTEN_PORT")
var HMACKey = os.Getenv("GH_HMAC_KEY")
var CmdRoot = os.Getenv("GH_CMD_ROOT")
var MaxChildRunTimeSeconds = 90
var CmdExtensions []string

func validMAC(message, messageMAC, key []byte) bool {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	expectedMAC := mac.Sum(nil)
	return hmac.Equal(messageMAC, expectedMAC)
}

func sendResponse(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)

	fmt.Fprint(w, message)
}

func parseSig(s string) []byte {
	sa := strings.Split(s, "sha256=")

	if len(sa) != 2 || len(sa[0]) != 0 {
		return nil
	}

	ab, err := hex.DecodeString(sa[1])

	if err == nil {
		return ab
	}

	return nil
}

func execHookCmd(cmd string, data []byte) bool {
	if _, err := os.Stat(cmd); err != nil {
		return false
	}

	ctx, _ := context.WithTimeout(context.Background(), time.Duration(MaxChildRunTimeSeconds*int(time.Second)))

	cc := exec.CommandContext(ctx, cmd)
	cc.Stdin = bytes.NewReader(data)
	cc.Stdout = os.Stdout
	cc.Stderr = os.Stderr

	log.Printf("running %s\n", cmd)
	if err := cc.Start(); err != nil {
		log.Printf("error running command '%s': %v\n", cmd, err.Error())

		return false
	}

	log.Printf("command '%s' executing\n", cmd)

	go func() {
		cc.Wait()
		log.Printf("command '%s' completed", cmd)
	}()

	return true
}

func handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		sendResponse(w, http.StatusBadRequest, "missing request body")

		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 25*1024*1024)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading body: %v\n", err)
		sendResponse(w, http.StatusBadRequest, "bad request body")
		return
	}

	hsh := r.Header.Get("X-Hub-Signature-256")
	if hsh == "" {
		log.Println("missing X-Hub-Signature-256 header")
		sendResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sig := parseSig(hsh)
	if sig == nil {
		log.Printf("bad X-Hub-Signature-256 header %v\n", hsh)
		sendResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if !validMAC(data, sig, []byte(HMACKey)) {
		log.Printf("bad X-Hub-Signature-256 header %v\n", hsh)
		sendResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body any
	if err = json.Unmarshal(data, &body); err != nil {
		log.Printf("unable to unmarshal body: %v", err)
		sendResponse(w, http.StatusBadRequest, "bad request body")
		return
	}

	cmd := ""
	if bm, ok := body.(map[string]any); ok {
		ref := bm["ref"].(string)

		if rm, ok := bm["repository"].(map[string]any); ok {
			if fn, ok := rm["full_name"].(string); ok {
				cmd = filepath.Join(CmdRoot, filepath.Clean(fn), filepath.Clean(ref))
			}
		}
	}

	if cmd == "" {
		log.Printf("bad request body format: %v", body)
		sendResponse(w, http.StatusBadRequest, "bad request body")
		return
	}

	log.Printf("executing command %v", cmd)

	cmdStarted := execHookCmd(cmd, data)
	if !cmdStarted {
		for _, ext := range CmdExtensions {
			cmdStarted = execHookCmd(cmd+ext, data)
			if cmdStarted {
				break
			}
		}
	}

	if !cmdStarted {
		log.Printf("unable to locate command '%s' (tried extensions %v)", cmd, CmdExtensions)
	}

	sendResponse(w, http.StatusOK, "hook processed")
}

func initConfig() {
	if ListenPort == "" {
		ListenPort = ":4040"
	}

	if CmdRoot == "" {
		CmdRoot = "/etc/githooker"
	}

	if HMACKey == "" {
		log.Fatal("no GH_HMAC_KEY environment variable set (github hook secret)")
	}

	if rts := os.Getenv("GH_MAX_RUN_SECS"); rts != "" {
		if i, err := strconv.Atoi(rts); err == nil {
			MaxChildRunTimeSeconds = i
		} else {
			log.Printf("warning: GH_MAX_RUN_SECS is not a valid integer, ignoring (%s)", rts)
		}
	}

	if cel := os.Getenv("GH_CMD_EXTENSIONS"); cel != "" {
		CmdExtensions = strings.Split(cel, " ")
	}
}

func main() {
	initConfig()

	http.HandleFunc("/", handleHook)

	log.Printf("githooker: initialized")
	log.Fatal(http.ListenAndServe(ListenPort, nil))
}
