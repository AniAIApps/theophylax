package main

import (
	"bytes"
	"crypto/ed25519"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"theophylax/internal/receipts"
)

// verifyReceiptCLI verifies a Proof-of-Inference receipt offline.
//
//	theophylax verify-receipt --keys https://gw.example.com/.well-known/theophylax-receipt-keys.json \
//	    --request req.json --response resp.json <jws | @file>
//
// A streamed response can be passed exactly as captured: its trailing
// ": theophylax-receipt <jws>" comment is not covered by the digest, so it is
// stripped before hashing, and its JWS is used when none is given.
func verifyReceiptCLI(args []string) int {
	fs := flag.NewFlagSet("verify-receipt", flag.ContinueOnError)
	keys := fs.String("keys", "", "JWKS URL or file with the gateway's receipt public keys")
	reqF := fs.String("request", "", "file containing the exact request body that was sent")
	respF := fs.String("response", "", "file containing the exact response body that was received")
	usage := func() int {
		fmt.Fprintln(os.Stderr, "usage: theophylax verify-receipt --keys <jwks-url|file> [--request f] [--response f] [<jws|@file>]")
		fmt.Fprintln(os.Stderr, "       the receipt may be omitted when --response is a captured stream ending in its receipt comment")
		return 2
	}
	if err := fs.Parse(args); err != nil || fs.NArg() > 1 || *keys == "" {
		return usage()
	}
	var respBody []byte
	trailerJWS := ""
	if *respF != "" {
		b, err := os.ReadFile(*respF)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		respBody = b
		if body, j, ok := splitReceiptTrailer(b); ok {
			respBody, trailerJWS = body, j
		}
	}
	jws := trailerJWS
	if fs.NArg() == 1 {
		jws = fs.Arg(0)
	} else if jws == "" {
		return usage()
	}
	if strings.HasPrefix(jws, "@") {
		b, err := os.ReadFile(jws[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		jws = strings.TrimSpace(string(b))
	}
	var jwks []byte
	var err error
	if strings.HasPrefix(*keys, "http") {
		c := &http.Client{Timeout: 10 * time.Second}
		resp, e := c.Get(*keys)
		if e != nil {
			fmt.Fprintln(os.Stderr, "fetch keys:", e)
			return 2
		}
		jwks, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	} else {
		jwks, err = os.ReadFile(*keys)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	set, err := receipts.ParseJWKS(jwks)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse keys:", err)
		return 2
	}
	rc, err := receipts.Verify(jws, map[string]ed25519.PublicKey(set))
	if err != nil {
		fmt.Println("✗ INVALID:", err)
		return 1
	}
	ok := true
	fmt.Printf("✓ signature valid (issuer %s)\n  request  %s\n  model    %s → %s via %s (%d attempt(s))\n  status   %d   tokens in/out %d/%d   cost $%.6f\n  policies %d in force, %d action(s)\n",
		rc.Issuer, rc.ID, rc.Requested, rc.Route.UpstreamModel, rc.Route.ProviderKind, rc.Route.Attempts, rc.Status, rc.Usage.Input, rc.Usage.Output, rc.CostUSD, len(rc.Policies), len(rc.Actions))
	if trailerJWS != "" {
		if trailerJWS == jws {
			fmt.Println("  (receipt comment removed from the captured stream before hashing)")
		} else {
			fmt.Println("  (captured stream carries a different receipt; its comment was removed before hashing)")
		}
	}
	for _, c := range []struct {
		name, file, digest string
		body               []byte
	}{{"request", *reqF, rc.ReqSHA256, nil}, {"response", *respF, rc.RespSHA256, respBody}} {
		if c.file == "" {
			continue
		}
		b := c.body
		if b == nil {
			var err error
			if b, err = os.ReadFile(c.file); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
		}
		name := c.name
		if receipts.SHA256Hex(b) == c.digest {
			fmt.Printf("✓ %s body matches receipt digest\n", name)
		} else {
			fmt.Printf("✗ %s body does NOT match receipt digest\n", name)
			ok = false
		}
	}
	if !ok {
		return 1
	}
	return 0
}

// receiptTrailer starts the SSE comment the gateway appends to a stream.
const receiptTrailer = ": theophylax-receipt "

// splitReceiptTrailer removes a final ": theophylax-receipt <jws>" line from a
// captured stream. It returns the bytes the receipt's digest covers (everything
// before that line) and the JWS, or ok=false if b does not end with one.
func splitReceiptTrailer(b []byte) (body []byte, jws string, ok bool) {
	t := bytes.TrimRight(b, "\r\n")
	start := bytes.LastIndexByte(t, '\n') + 1 // 0 when the trailer is the only line
	line := t[start:]
	if !bytes.HasPrefix(line, []byte(receiptTrailer)) {
		return b, "", false
	}
	jws = strings.TrimSpace(string(line[len(receiptTrailer):]))
	if jws == "" {
		return b, "", false
	}
	return b[:start], jws, true
}
