package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/go-kt/pmetrics"
	"github.com/katbyte/truenas-mcp/sdk/truenastest"
	"github.com/katbyte/truenas-metrics/lib/poll"
	"github.com/katbyte/truenas-metrics/lib/poll/polltest"
	"github.com/spf13/cobra"
)

// answers is where the saved answers of a NAS are kept, one file to a method. It is worked out before any test moves to a folder of its own.
var answers = func() string {
	dir, err := filepath.Abs("../lib/nas/testdata")
	if err != nil {
		panic(err)
	}

	return dir
}()

// stood is a connector that hands out this NAS whatever it is asked to connect to, and counts how many times its connection was closed.
func stood(nas *polltest.NAS, closed *int) connector {
	return func(connection) (poll.NAS, func(), error) {
		return nas, func() {
			if closed != nil {
				*closed++
			}
		}, nil
	}
}

// program is the program given these arguments and this NAS, not yet run, and where what it prints will go.
func program(t *testing.T, connect connector, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	root, err := makeWith(connect)
	if err != nil {
		t.Fatalf("makeWith: %v", err)
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)

	return root, &out
}

// run executes the program with these arguments against this NAS, and gives back what it printed.
func run(t *testing.T, connect connector, args ...string) (string, error) {
	t.Helper()

	root, out := program(t, connect, args...)
	err := root.ExecuteContext(t.Context())

	return out.String(), err
}

// to is the settings that name a NAS, a user and a key, followed by any others.
func to(command string, more ...string) []string {
	return append([]string{command, "--server", testServer, "--username", testUser, "--api-key", testKey}, more...)
}

//nolint:paralleltest // viper is global state; Make mutates it
func TestMakeBuildsTheCommands(t *testing.T) {
	alone(t)

	root, err := Make()
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"version", "print", "serve"} {
		if c, _, err := root.Find([]string{name}); err != nil || c.Name() != name {
			t.Errorf("no %s command: %v", name, err)
		}
	}
	// a setting is a flag on every command, so it can be given after whichever one is run
	for _, name := range []string{"server", "username", "api-key", "ca-file", "listen", "slow-interval", "dataset-depth", "config"} {
		if root.PersistentFlags().Lookup(name) == nil {
			t.Errorf("no --%s flag", name)
		}
	}
}

//nolint:paralleltest // viper is global state; Make mutates it
func TestVersionCommand(t *testing.T) {
	alone(t)

	out, err := run(t, stood(&polltest.NAS{}, nil), "version")
	if err != nil || !strings.HasPrefix(out, "truenas-metrics ") {
		t.Errorf("version printed %q (%v), want the name and a version", out, err)
	}
}

// With no command the program says where the help is rather than serving or failing.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestNoCommandPointsAtHelp(t *testing.T) {
	alone(t)

	out, err := run(t, stood(&polltest.NAS{}, nil))
	if err != nil || !strings.Contains(out, "truenas-metrics help") {
		t.Errorf("no command printed %q (%v), want a pointer to help", out, err)
	}
}

// print asks every question once and writes the page a scrape would read, then closes the connection it asked over.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestPrintWritesThePage(t *testing.T) {
	alone(t)

	nas := &polltest.NAS{PoolsAnswer: []poll.Pool{{Name: "tank", Status: "ONLINE", Healthy: true}}, DisksAnswer: []poll.Disk{{Name: "sda", Serial: "ZR5A0001"}}}
	var closed int

	out, err := run(t, stood(nas, &closed), to("print")...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`truenas_up 1`, `truenas_pool_healthy{pool="tank"} 1`, `truenas_disk_size_bytes{disk="sda",serial="ZR5A0001"} 0`, `truenas_poll_failures_total{poll="slow"} 0`} {
		if !strings.Contains(out, want) {
			t.Errorf("the page lacks %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, testKey) {
		t.Errorf("the page holds the key:\n%s", out)
	}
	// once each, not on a schedule
	if nas.Asked(polltest.Pools) != 1 || nas.Asked(polltest.Disks) != 1 {
		t.Errorf("print asked for pools %d times and disks %d, want once each", nas.Asked(polltest.Pools), nas.Asked(polltest.Disks))
	}
	if closed != 1 {
		t.Errorf("the connection was closed %d times, want once", closed)
	}
}

// Datasets are named as deep as the setting says and no deeper.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestDatasetDepthSaysHowDeepDatasetsAreNamed(t *testing.T) {
	nas := &polltest.NAS{DatasetsAnswer: []poll.Dataset{{Name: "tank", Pool: "tank"}, {Name: "tank/media", Pool: "tank"}, {Name: "tank/media/films", Pool: "tank"}}}

	for depth, named := range map[string][]bool{"0": {true, false, false}, "1": {true, true, false}, "2": {true, true, true}} {
		alone(t)

		out, err := run(t, stood(nas, nil), to("print", "--dataset-depth", depth)...)
		if err != nil {
			t.Fatal(err)
		}
		for i, dataset := range []string{"tank", "tank/media", "tank/media/films"} {
			if got := strings.Contains(out, `truenas_dataset_used_bytes{dataset="`+dataset+`",`); got != named[i] {
				t.Errorf("--dataset-depth %s: %s on the page is %t, want %t", depth, dataset, got, named[i])
			}
		}
		if !strings.Contains(out, `truenas_datasets{pool="tank"} 3`) {
			t.Errorf("--dataset-depth %s: the count of datasets is not all three:\n%s", depth, out)
		}
	}
}

// Labels asked for are on every reading, the one that says which build wrote the page included, beside the labels a reading has of its own.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestLabelsGoOnEveryReading(t *testing.T) {
	alone(t)

	nas := &polltest.NAS{PoolsAnswer: []poll.Pool{{Name: "tank", Healthy: true}}}

	out, err := run(t, stood(nas, nil), to("print", "--labels", "hostname=nas, site=home")...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`truenas_up{hostname="nas",site="home"} 1`, `truenas_pool_healthy{hostname="nas",pool="tank",site="home"} 1`, `truenas_metrics_build_info{commit="",goversion="`, `hostname="nas",site="home",version="`} {
		if !strings.Contains(out, want) {
			t.Errorf("the page lacks %s:\n%s", want, out)
		}
	}
	for line := range strings.Lines(out) {
		if !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" && !strings.Contains(line, `hostname="nas"`) {
			t.Errorf("a reading has no hostname label: %s", line)
		}
	}
}

// A label that cannot be used stops both commands before the NAS is asked anything, and the error says which label and which setting: one not
// written name=value, one a store would not take, and one named as a reading's own label is.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestALabelThatCannotBeUsed(t *testing.T) {
	for labels, want := range map[string]string{"hostname": "hostname", "9lives=cat": "9lives", "pool=tank": "pool", "hostname=": "hostname"} {
		for _, command := range []string{"print", "serve"} {
			alone(t)

			nas := &polltest.NAS{}
			var closed int

			_, err := run(t, stood(nas, &closed), to(command, "--listen", "127.0.0.1:0", "--labels", labels)...)
			if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "TRUENAS_METRICS_LABELS") {
				t.Errorf("%s --labels %s = %v, want an error naming %q and the setting", command, labels, err, want)
			}
			if asked := nas.Asked(polltest.Pools) + nas.Asked(polltest.Disks); asked != 0 {
				t.Errorf("%s --labels %s asked the NAS %d questions before refusing", command, labels, asked)
			}
			if closed != 1 {
				t.Errorf("%s --labels %s left the connection open", command, labels)
			}
		}
	}
}

// print is also the check that the address, the user and the key work, so a NAS that does not answer makes it fail, with the page still printed
// to show what that looks like. One question refused is a failure too, with the rest of the page still there.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestPrintFailsWhenTheNASDoesNotAnswer(t *testing.T) {
	alone(t)

	nas := &polltest.NAS{}
	nas.Fail("", errors.New("connection refused"))

	out, err := run(t, stood(nas, nil), to("print")...)
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Errorf("print with the NAS down = %v, want an error saying it did not answer", err)
	}
	if !strings.Contains(out, "truenas_up 0") || strings.Contains(out, "truenas_pool_") {
		t.Errorf("the page should show the NAS down and nothing of its pools:\n%s", out)
	}

	alone(t)

	nas = &polltest.NAS{PoolsAnswer: []poll.Pool{{Name: "tank", Healthy: true}}}
	nas.Fail(polltest.Tasks, errors.New("not authorized"))

	out, err = run(t, stood(nas, nil), to("print")...)
	if err == nil {
		t.Error("print with one question refused succeeded, want it to fail")
	}
	if !strings.Contains(out, "truenas_up 1") || !strings.Contains(out, `truenas_pool_healthy{pool="tank"} 1`) || !strings.Contains(out, `truenas_poll_failures_total{poll="slow"} 1`) {
		t.Errorf("the page should show the NAS up, its pool, and the slow round failed:\n%s", out)
	}
}

// With no NAS, user or key named there is nothing to ask, and both commands say which variable names it rather than serve an empty page.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestACommandNeedsANASAUserAndAKey(t *testing.T) {
	for _, command := range []string{"print", "serve"} {
		for given, missing := range map[string]string{
			"":                                   "TRUENAS_METRICS_SERVER",
			"--server nas":                       "TRUENAS_METRICS_USERNAME",
			"--server nas --username metrics":    "TRUENAS_METRICS_API_KEY",
			"--username metrics --api-key 3-key": "TRUENAS_METRICS_SERVER",
		} {
			alone(t)

			_, err := run(t, stood(&polltest.NAS{}, nil), append([]string{command}, strings.Fields(given)...)...)
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Errorf("%s %s = %v, want an error naming %s", command, given, err, missing)
			}
		}
	}
}

// freePort is an address on this machine nothing else is listening on: taken, noted and given back.
func freePort(t *testing.T) string {
	t.Helper()

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	return ln.Addr().String()
}

// fetch reads a path from the program once it is serving, trying for a second while it starts.
func fetch(t *testing.T, addr, path string) string {
	t.Helper()

	for range 50 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		return string(body)
	}
	t.Fatalf("nothing answered at %s%s", addr, path)

	return ""
}

// serving runs serve with these arguments until the test ends it, and gives back what stops it and where its result arrives.
func serving(t *testing.T, connect connector, args ...string) (stop context.CancelFunc, stopped <-chan error) {
	t.Helper()

	root, _ := program(t, connect, args...)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()

	return cancel, done
}

// served waits for the page to hold every one of these readings, for a second, and gives back the page as it last was.
func served(t *testing.T, addr string, want ...string) string {
	t.Helper()

	var body string
	for range 50 {
		body = fetch(t, addr, pmetrics.Path)

		missing := false
		for _, w := range want {
			missing = missing || !strings.Contains(body, w)
		}
		if !missing {
			return body
		}
		time.Sleep(20 * time.Millisecond)
	}

	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("the page lacks %s", w)
		}
	}
	t.Logf("the page:\n%s", body)

	return body
}

// serve is the whole program: it polls the NAS, serves what it said, and stops cleanly when told to, closing the connection behind it.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestServe(t *testing.T) {
	alone(t)

	nas := &polltest.NAS{PoolsAnswer: []poll.Pool{{Name: "tank", Healthy: true}}, DisksAnswer: make([]poll.Disk, 3)}
	nas.Fail(polltest.Tasks, errors.New("not authorized"))
	addr := freePort(t)
	var closed int

	stop, stopped := serving(t, stood(nas, &closed), to("serve", "--listen", addr)...)

	if got := strings.TrimSpace(fetch(t, addr, pmetrics.HealthPath)); got != "ok" {
		t.Errorf("%s = %q, want ok", pmetrics.HealthPath, got)
	}

	// the first rounds are asked as the program starts, so they are on the page within moments: one question refused costs its own readings only
	body := served(t, addr, `truenas_up 1`, `truenas_pool_healthy{pool="tank"} 1`, `truenas_poll_failures_total{poll="slow"} 1`, `truenas_alerts{level="CRITICAL"} 0`)
	if strings.Contains(body, testKey) || strings.Contains(body, "truenas_tasks") {
		t.Errorf("the page holds the key, or tasks the NAS refused to list:\n%s", body)
	}

	stop()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("serve stopped with %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop when told to")
	}
	if closed != 1 {
		t.Errorf("the connection was closed %d times, want once", closed)
	}
}

// A port that is already taken stops the program with the address in the error, and the polling it had started stops with it.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestServeSaysWhenThePortIsTaken(t *testing.T) {
	alone(t)

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()

	_, stopped := serving(t, stood(&polltest.NAS{}, nil), to("serve", "--listen", addr)...)

	select {
	case err := <-stopped:
		if err == nil || !strings.Contains(err.Error(), addr) {
			t.Errorf("serve on a taken port = %v, want an error naming %s", err, addr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not give up on a port that was taken")
	}
}

// standIn is a NAS that speaks the API as a real one does, over TLS and behind a login, and answers each method the exporter calls with the answer
// saved for it. Like a TrueNAS as it comes, it shows a certificate it made itself, made out to localhost, though it is reached at another address.
// The certificate is written to a file, which is how a real NAS's is handed to the program.
func standIn(t *testing.T) (nas *truenastest.NAS, certFile string) {
	t.Helper()

	nas = truenastest.New(t, truenastest.WithCertificate(truenastest.SelfSigned(t, nil, "localhost", time.Now().Add(24*time.Hour))))

	files, err := filepath.Glob(filepath.Join(answers, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no saved answers in %s: %v", answers, err)
	}
	for _, file := range files {
		answer, err := os.ReadFile(file) //nolint:gosec // G304: the file is one of the saved answers the test found itself
		if err != nil {
			t.Fatal(err)
		}
		nas.Answer(strings.TrimSuffix(filepath.Base(file), ".json"), json.RawMessage(answer))
	}

	certFile = filepath.Join(t.TempDir(), "nas.pem")
	if err := os.WriteFile(certFile, nas.CertificatePEM(), 0o600); err != nil {
		t.Fatal(err)
	}

	return nas, certFile
}

// The program as it is shipped, against a NAS that speaks the real protocol: it logs in with the user and the key over TLS, knowing the NAS by
// the certificate it was given, asks only for what it reads, and the page holds what the saved answers say. The key is on the page nowhere.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestPrintAgainstANASThatSpeaksTheAPI(t *testing.T) {
	alone(t)

	nas, certFile := standIn(t)

	root, err := Make()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"print", "--server", nas.Address(), "--username", nas.Username(), "--api-key", nas.Key(), "--certificate-file", certFile, "--dataset-depth", "2"})

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("print: %v\n%s", err, out.String())
	}

	page := out.String()
	for _, want := range []string{
		`truenas_up 1`,
		`truenas_system_info{cpu="AMD EPYC 7543 32-Core Processor",manufacturer="QEMU",product="Standard PC (Q35 + ICH9, 2009)",version="25.10.7"} 1`,
		`truenas_system_state{state="READY"} 1`,
		`truenas_pool_healthy{pool="tank"} 1`, `truenas_pool_healthy{pool="boot-pool"} 1`, `truenas_pool_fragmentation_ratio{pool="tank"} 0.14`,
		`truenas_pool_scan_running{function="SCRUB",pool="tank"} 0`, `truenas_pool_scan_progress_ratio{function="SCRUB",pool="tank"} 1`,
		`truenas_pool_vdev_info{class="data",pool="tank",type="RAIDZ2",vdev="raidz2-0"} 1`,
		// the pool names the disk and the list of disks gives its serial number
		`truenas_pool_device_status{class="data",disk="sdf",pool="tank",serial="ZR5A0004",status="ONLINE",vdev="raidz2-0"} 1`,
		`truenas_pool_device_checksum_errors{class="spare",disk="sdae",pool="tank",serial="ZR5A0010",vdev="sdae"} 0`,
		`truenas_disk_info{bus="SCSI",disk="sdj",enclosure="",model="WUH722020BL5204",pool="tank",serial="ZR5A0001",slot="",type="HDD",wwn="5000c500e0000001"} 1`,
		`truenas_disk_temperature_celsius{disk="nvme1n1",serial="ZR5A0009"} 46.85`,
		`truenas_datasets{pool="tank"} 6`, `truenas_dataset_snapshots{dataset="tank/dataset-5",pool="tank"} 12`, `truenas_dataset_compression_ratio{dataset="tank",pool="tank"} 1.04`,
		`truenas_alerts{level="CRITICAL"} 2`, `truenas_alerts_by_class{class="RsyncFailed",level="CRITICAL"} 2`,
		`truenas_service_running{service="cifs"} 1`, `truenas_jobs_running{method="cloudsync.sync"} 1`,
		`truenas_apps_service_status{status="UNCONFIGURED"} 1`, `truenas_vms{state="RUNNING"} 1`,
		`truenas_tasks{kind="rsync"} 4`, `truenas_tasks_failed{kind="rsync"} 2`, `truenas_task_running{id="5",kind="cloud_sync",name="description-3"} 1`,
		`truenas_shares{protocol="smb",state="enabled"} 3`, `truenas_shares{protocol="nfs",state="locked"} 1`,
		`truenas_certificate_expiry_timestamp_seconds{certificate="truenas_default"} 1.8043218e+09`,
		`truenas_interface_link_up{interface="enp1s0",type="PHYSICAL"} 1`, `truenas_update_available 0`, `truenas_api_key_expiry_timestamp_seconds 1.822536e+09`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	if t.Failed() {
		t.Logf("the page:\n%s", page)
	}

	if strings.Contains(page, nas.Key()) {
		t.Error("the page holds the key")
	}
	if nas.Dials() != 1 || nas.Logins() != 1 {
		t.Errorf("the NAS was dialled %d times and logged into %d, want one connection for every question", nas.Dials(), nas.Logins())
	}

	// nothing is called but the login, what the connection asks of itself, and the reads the saved answers are of
	for _, call := range nas.Calls() {
		if _, err := os.Stat(filepath.Join(answers, call.Method+".json")); err != nil && !strings.HasPrefix(call.Method, "auth.") && !strings.HasPrefix(call.Method, "core.") {
			t.Errorf("the NAS was asked for %s, which is not one of the reads", call.Method)
		}
	}
}

// A key the NAS will not take, a certificate nobody vouched for, the NAS's certificate handed over as an authority when the name on it is not the
// NAS's, and a certificate that is some other machine's: each way print fails, says the NAS is down on the page, and the key is in neither what
// it printed nor what it said went wrong.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestPrintAgainstANASThatWillNotLetItIn(t *testing.T) {
	for name, args := range map[string]func(nas *truenastest.NAS, certFile, otherFile string) []string{
		"a wrong key": func(_ *truenastest.NAS, certFile, _ string) []string {
			return []string{"--api-key", "9-wr0ngk3y", "--certificate-file", certFile}
		},
		"an unknown certificate": func(nas *truenastest.NAS, _, _ string) []string {
			return []string{"--api-key", nas.Key()}
		},
		"a certificate made out to another name, as an authority": func(nas *truenastest.NAS, certFile, _ string) []string {
			return []string{"--api-key", nas.Key(), "--ca-file", certFile}
		},
		"another machine's certificate": func(nas *truenastest.NAS, _, otherFile string) []string {
			return []string{"--api-key", nas.Key(), "--certificate-file", otherFile}
		},
	} {
		alone(t)

		nas, certFile := standIn(t)
		_, otherFile := standIn(t)

		root, err := Make()
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(io.Discard)
		root.SetArgs(append([]string{"print", "--server", nas.Address(), "--username", nas.Username()}, args(nas, certFile, otherFile)...))

		err = root.ExecuteContext(t.Context())
		if err == nil {
			t.Errorf("%s: print succeeded, want it to fail", name)
		}
		if page := out.String(); !strings.Contains(page, "truenas_up 0") || strings.Contains(page, "truenas_pool_") || strings.Contains(page, "wr0ngk3y") || strings.Contains(page, nas.Key()) {
			t.Errorf("%s: the page should say the NAS is down and nothing else, and hold no key:\n%s", name, page)
		}
		if err != nil && (strings.Contains(err.Error(), "wr0ngk3y") || strings.Contains(err.Error(), nas.Key())) {
			t.Errorf("%s: the error holds the key: %v", name, err)
		}
		// a NAS that is not known to be the NAS is never sent the key
		if name != "a wrong key" && nas.Logins() != 0 {
			t.Errorf("%s: the NAS was logged into %d times, want the key never sent", name, nas.Logins())
		}
	}
}

// printed runs print as it is shipped with these settings, and gives back the page and how it ended.
func printed(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root, err := Make()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"print"}, args...))

	err = root.ExecuteContext(t.Context())

	return out.String(), err
}

// Skipping the check is the setting of last resort, and by itself it does not hand over the key: a TrueNAS before version 26 takes a key only as
// it is, so whatever answered at its address would be given it. It is not logged into until a second setting says to send the key all the same.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestSkippingTheCertificateCheckDoesNotHandOverTheKeyUnlessTold(t *testing.T) {
	alone(t)

	nas, certFile := standIn(t)
	reach := []string{"--server", nas.Address(), "--username", nas.Username(), "--api-key", nas.Key()}

	page, err := printed(t, append(reach, "--skip-certificate-check")...)
	if err == nil || !strings.Contains(page, "truenas_up 0") || nas.Logins() != 0 {
		t.Errorf("print --skip-certificate-check = %v after %d logins, want it to fail with the NAS down and the key never sent:\n%s", err, nas.Logins(), page)
	}

	alone(t)

	page, err = printed(t, append(reach, "--skip-certificate-check", "--send-key-unchecked")...)
	if err != nil || !strings.Contains(page, "truenas_up 1") || nas.Logins() != 1 {
		t.Errorf("print --skip-certificate-check --send-key-unchecked = %v after %d logins, want the NAS reached:\n%s", err, nas.Logins(), page)
	}

	// said without the setting it goes with, or with a second way of trusting the certificate, the settings are refused before the NAS is dialled
	dials := nas.Dials()
	for _, wrong := range [][]string{{"--send-key-unchecked"}, {"--skip-certificate-check", "--certificate-file", certFile}} {
		alone(t)

		if _, err := printed(t, append(reach, wrong...)...); err == nil || !strings.Contains(err.Error(), "TRUENAS_METRICS_SKIP_CERTIFICATE_CHECK") || nas.Dials() != dials {
			t.Errorf("print %v = %v after %d more dials, want it refused naming the settings and the NAS not dialled", wrong, err, nas.Dials()-dials)
		}
	}
}

// A TrueNAS from version 26 on is logged into without the key being sent at all. The same settings reach it, and nothing it is sent holds the key.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestPrintAgainstATrueNAS26(t *testing.T) {
	alone(t)

	nas := truenastest.New(t, truenastest.Version26(), truenastest.WithCertificate(truenastest.SelfSigned(t, nil, "localhost", time.Now().Add(24*time.Hour))))
	nas.Answer("system.info", json.RawMessage(`{"version": "26.0.0"}`))
	nas.Answer("system.state", "READY")

	certFile := filepath.Join(t.TempDir(), "nas.pem")
	if err := os.WriteFile(certFile, nas.CertificatePEM(), 0o600); err != nil {
		t.Fatal(err)
	}

	// the stand-in answers only the two methods it was given, so the rounds fail on the rest: what matters here is that it was logged into
	page, _ := printed(t, "--server", nas.Address(), "--username", nas.Username(), "--api-key", nas.Key(), "--certificate-file", certFile)
	if nas.Logins() != 1 {
		t.Fatalf("a TrueNAS 26 was logged into %d times, want once:\n%s", nas.Logins(), page)
	}

	for _, call := range nas.Calls() {
		sent, err := json.Marshal(call.Params)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(sent), nas.Key()) {
			t.Errorf("%s was sent the key, which a TrueNAS 26 is never sent", call.Method)
		}
	}
}

// The NAS revokes a key sent to it in the clear, so an address that is not TLS is refused before anything is sent to it.
//
//nolint:paralleltest // viper is global state; Make mutates it
func TestAnAddressThatIsNotTLSIsRefused(t *testing.T) {
	for _, address := range []string{"http://nas.example.net", "ws://nas.example.net/api/current"} {
		alone(t)

		root, err := Make()
		if err != nil {
			t.Fatal(err)
		}
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"print", "--server", address, "--username", testUser, "--api-key", testKey})

		if err := root.ExecuteContext(t.Context()); err == nil || strings.Contains(err.Error(), testKey) {
			t.Errorf("print --server %s = %v, want it refused without the key in the error", address, err)
		}
	}
}
