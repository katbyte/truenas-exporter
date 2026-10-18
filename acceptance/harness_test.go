//go:build acceptance

package acceptance_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/katbyte/truenas-exporter/cli"
	"github.com/katbyte/truenas-mcp/sdk/truenasrpc"
)

// The NAS under test and how to log in to it, from the environment. They are variables so that a test of the suite itself can stand a NAS in.
var (
	address  = os.Getenv("TRUENAS_EXPORTER_TEST_SERVER")
	username = os.Getenv("TRUENAS_EXPORTER_TEST_USERNAME")
	key      = os.Getenv("TRUENAS_EXPORTER_TEST_API_KEY")
	certFile = os.Getenv("TRUENAS_EXPORTER_TEST_CERTIFICATE_FILE")
)

// furnished is whether the NAS under test is truenas-mcp's throwaway one with one of each thing a NAS is watched for on it, under the names its
// layout package gives them. make testacc asks its test environment for such a NAS with this same variable, which is how the suite knows.
var furnished = os.Getenv("TRUENAS_TEST_FURNISHED") == "1"

// patience is how long the page is given to show what a poll of the NAS brought back.
const patience = 30 * time.Second

// live skips a test when no NAS is named.
func live(t *testing.T) {
	t.Helper()

	if address == "" || username == "" || key == "" {
		t.Skip("no TrueNAS to test against: set TRUENAS_EXPORTER_TEST_SERVER, TRUENAS_EXPORTER_TEST_USERNAME and TRUENAS_EXPORTER_TEST_API_KEY")
	}
}

// deployed gives the program the NAS under test the way a deployment does, in its environment, where no settings file or variable of the
// developer's own can reach it. The page names datasets however deep, so that every one the NAS lists can be looked for.
func deployed(t *testing.T) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("TRUENAS_EXPORTER_CONFIG", "")
	for _, unset := range []string{"CA_FILE", "SKIP_CERTIFICATE_CHECK", "SEND_KEY_UNCHECKED", "LISTEN", "LABELS", "FAST_INTERVAL", "SLOW_INTERVAL"} {
		t.Setenv("TRUENAS_EXPORTER_"+unset, "")
	}

	t.Setenv("TRUENAS_EXPORTER_SERVER", address)
	t.Setenv("TRUENAS_EXPORTER_USERNAME", username)
	t.Setenv("TRUENAS_EXPORTER_API_KEY", key)
	t.Setenv("TRUENAS_EXPORTER_CERTIFICATE_FILE", certFile)
	t.Setenv("TRUENAS_EXPORTER_DATASET_DEPTH", "64")
}

// printed runs the program's print as it is shipped against the NAS under test, and gives back the page and how it ended.
func printed(t *testing.T) (string, error) {
	t.Helper()

	deployed(t)

	cmd, err := cli.Make()
	if err != nil {
		t.Fatalf("making the command: %v", err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"print"})

	err = cmd.ExecuteContext(t.Context())

	return out.String(), err
}

// serving starts the program serving on a port of its own, asking about the parts of the NAS as often as intervals says, and gives back where it is. It is told to stop when the test
// ends, and the test fails if it does not stop cleanly.
func serving(t *testing.T, intervals string) string {
	t.Helper()

	deployed(t)

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	at := listener.Addr().String()
	_ = listener.Close()

	cmd, err := cli.Make()
	if err != nil {
		t.Fatalf("making the command: %v", err)
	}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"serve", "--listen", at, "--intervals", intervals})

	ctx, stop := context.WithCancel(context.WithoutCancel(t.Context()))
	stopped := make(chan error, 1)
	go func() { stopped <- cmd.ExecuteContext(ctx) }()

	t.Cleanup(func() {
		stop()

		select {
		case err := <-stopped:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("serve stopped with %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("serve did not stop when told to")
		}
	})

	return "http://" + at
}

// asking is a connection of the suite's own to the NAS under test, for asking it directly what the page should say. The program never uses it: it
// makes its own.
func asking(t *testing.T) *truenasrpc.Conn {
	t.Helper()

	var opts []truenasrpc.Option
	if certFile != "" {
		opts = append(opts, truenasrpc.WithCertificateFile(certFile))
	}

	conn, err := truenasrpc.New(address, truenasrpc.APIKey(username, key), opts...)
	if err != nil {
		t.Fatalf("connecting to the NAS under test: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

// shown is the certificate the NAS under test shows, from the file the suite was given for it.
func shown(t *testing.T) *x509.Certificate {
	t.Helper()

	text, err := os.ReadFile(certFile) //nolint:gosec // G304: the file is the one the person running the suite named for the NAS's certificate
	if err != nil {
		t.Fatalf("reading the NAS's certificate: %v", err)
	}

	block, _ := pem.Decode(text)
	if block == nil {
		t.Fatalf("%s holds no certificate", certFile)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("reading the NAS's certificate: %v", err)
	}

	return cert
}

// asked calls one method on the NAS under test and gives its answer as a T.
func asked[T any](t *testing.T, conn *truenasrpc.Conn, method string, params ...any) T {
	t.Helper()

	var answer T
	if err := conn.Call(t.Context(), method, params, &answer); err != nil {
		t.Fatalf("asking the NAS under test for %s: %v", method, err)
	}

	return answer
}
