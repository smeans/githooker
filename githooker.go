package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var DebugMode = os.Getenv("GH_DEBUG") != ""
var ListenPort = os.Getenv("GH_LISTEN_PORT")
var HMACKey = os.Getenv("GH_HMAC_KEY")

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

	var body interface{}
	if err = json.Unmarshal(data, &body); err != nil {
		log.Printf("unable to unmarshal body: %v", err)
		sendResponse(w, http.StatusBadRequest, "bad request body")
		return
	}

	fmt.Print(body)

	sendResponse(w, http.StatusOK, "hook processed")
}

func main() {
	if ListenPort == "" {
		ListenPort = ":4040"
	}

	if HMACKey == "" {
		log.Fatal("no GH_HMAC_KEY environment variable set (github hook secret)")
	}

	http.HandleFunc("/", handleHook)

	log.Fatal(http.ListenAndServe(ListenPort, nil))
}
