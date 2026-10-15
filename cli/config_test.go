package cli

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/truenas-exporter/lib/poll"
	"github.com/katbyte/truenas-exporter/lib/poll/polltest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// viper is global, so none of these can run in parallel and each resets it.
const (
	testUse    = "truenas-exporter"
	testServer = "nas.example.net"
	testUser   = "metrics"
	testKey    = "3-s3cretk3y"
)

// alone cuts a test off from the settings of whoever runs it: viper as it starts, an empty home and working directory, and none of the program's
// variables set.
func alone(t *testing.T) (home, wd string) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	home, wd = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(wd)
	t.Setenv(configEnv, "")
	for _, env := range envNames {
		t.Setenv(env, "")
	}

	return home, wd
}

// load drives the real flag wiring with these command-line arguments, after whatever the test put in the environment and the settings files.
func load(t *testing.T, args ...string) (*FlagData, error) {
	t.Helper()

	root := &cobra.Command{Use: testUse}
	if err := configureFlags(root); err != nil {
		t.Fatalf("configureFlags: %v", err)
	}
	if err := root.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := loadConfig(); err != nil {
		return nil, err
	}

	return GetFlags(), nil
}

// must is load for a test that expects the settings to be read.
func must(t *testing.T, args ...string) *FlagData {
	t.Helper()

	f, err := load(t, args...)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	return f
}

func write(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// With nothing set the page is served where the README says, the NAS is asked as often as it says, and datasets are named as deep as it says:
// these are what a dashboard's scrape interval and an example compose file are written against.
//
//nolint:paralleltest // viper is global state; these mutate it
func TestDefaults(t *testing.T) {
	alone(t)

	f := must(t)
	every, err := f.intervals()
	if err != nil || f.Listen != ":9089" || every[poll.AreaSystem] != 30*time.Second || every[poll.AreaDisks] != 5*time.Minute || every[poll.AreaSnapshots] != 10*time.Minute || f.DatasetDepth != 1 || f.ClockCorrection != 500*time.Millisecond {
		t.Errorf("defaults = %s, every %s, %s and %s (%v), datasets %d deep, the clock corrected by %s; want :9089, 30s, 5m, 10m, 1 and 500ms", f.Listen, every[poll.AreaSystem], every[poll.AreaDisks], every[poll.AreaSnapshots], err, f.DatasetDepth, f.ClockCorrection)
	}
	if f.Server != "" || f.Username != "" || f.APIKey != "" || f.CertificateFile != "" || f.CAFile != "" || f.SkipCertificateCheck || f.SendKeyUnchecked || f.Labels != "" || f.Metrics != "" || f.Skip != "" || f.Intervals != "" {
		t.Errorf("a NAS, a certificate, a label, a part of the NAS to leave out or an interval is set with nothing given: %+v", f)
	}
}

// Every setting has an environment variable, which is how a container is configured.
func TestEveryFlagHasAnEnvironmentVariable(t *testing.T) {
	alone(t)

	t.Setenv("TRUENAS_EXPORTER_SERVER", testServer)
	t.Setenv("TRUENAS_EXPORTER_USERNAME", testUser)
	t.Setenv("TRUENAS_EXPORTER_API_KEY", testKey)
	t.Setenv("TRUENAS_EXPORTER_CERTIFICATE_FILE", "/etc/nas.pem")
	t.Setenv("TRUENAS_EXPORTER_CA_FILE", "/etc/authority.pem")
	t.Setenv("TRUENAS_EXPORTER_SKIP_CERTIFICATE_CHECK", "true")
	t.Setenv("TRUENAS_EXPORTER_SEND_KEY_UNCHECKED", "true")
	t.Setenv("TRUENAS_EXPORTER_LISTEN", "127.0.0.1:9000")
	t.Setenv("TRUENAS_EXPORTER_LABELS", "hostname=nas")
	t.Setenv("TRUENAS_EXPORTER_DATASET_DEPTH", "3")
	t.Setenv("TRUENAS_EXPORTER_METRICS", "pools,disks")
	t.Setenv("TRUENAS_EXPORTER_SKIP", "disks")
	t.Setenv("TRUENAS_EXPORTER_INTERVALS", "snapshots=30m")
	t.Setenv("TRUENAS_EXPORTER_CLOCK_CORRECTION", "250ms")

	want := FlagData{
		Server: testServer, Username: testUser, APIKey: testKey, CertificateFile: "/etc/nas.pem", CAFile: "/etc/authority.pem", SkipCertificateCheck: true, SendKeyUnchecked: true,
		Listen: "127.0.0.1:9000", Labels: "hostname=nas", DatasetDepth: 3, Metrics: "pools,disks", Skip: "disks", Intervals: "snapshots=30m", ClockCorrection: 250 * time.Millisecond,
	}
	if got := *must(t); got != want {
		t.Errorf("from the environment = %+v, want %+v", got, want)
	}

	root := &cobra.Command{Use: testUse}
	if err := configureFlags(root); err != nil {
		t.Fatal(err)
	}
	root.PersistentFlags().VisitAll(func(fl *pflag.Flag) {
		if _, ok := envNames[fl.Name]; !ok && fl.Name != "config" {
			t.Errorf("--%s has no environment variable", fl.Name)
		}
	})
}

// A flag beats the environment, which beats a settings file: a container overrides what an image holds, and a command line overrides both.
func TestFlagThenEnvironmentThenFile(t *testing.T) {
	home, _ := alone(t)
	write(t, home, ".truenas-exporter", "SERVER=from-file\nAPI_KEY=from-file\nINTERVALS=snapshots=30m\nSKIP_CERTIFICATE_CHECK=true\nDATASET_DEPTH=2\nLISTEN=:9000\n")

	f := must(t)
	if f.Server != "from-file" || f.APIKey != "from-file" || f.Intervals != "snapshots=30m" || !f.SkipCertificateCheck || f.DatasetDepth != 2 || f.Listen != ":9000" {
		t.Errorf("from the file = %+v, want every setting it holds, the ones of several words included", f)
	}

	viper.Reset()
	t.Setenv("TRUENAS_EXPORTER_SERVER", "from-env")
	t.Setenv("TRUENAS_EXPORTER_INTERVALS", "apps=1m")
	f = must(t)
	if f.Server != "from-env" || f.Intervals != "apps=1m" || f.APIKey != "from-file" {
		t.Errorf("with the environment = %+v, want it over the file, and the file for the rest", f)
	}

	viper.Reset()
	f = must(t, "--server", "from-flag", "--intervals", "system=10s")
	if f.Server != "from-flag" || f.Intervals != "system=10s" {
		t.Errorf("with flags = %+v, want them over the environment", f)
	}
}

// A .truenas-exporter in the working directory is found before the one in $HOME, and neither being there is not an error.
//
//nolint:paralleltest // viper is global state; these mutate it
func TestSettingsFilePrecedence(t *testing.T) {
	home, wd := alone(t)

	if f := must(t); f.Server != "" {
		t.Errorf("server = %q with no settings file, want none", f.Server)
	}

	viper.Reset()
	write(t, home, ".truenas-exporter", "SERVER=from-home\n")
	if f := must(t); f.Server != "from-home" {
		t.Errorf("server = %q, want the one in $HOME", f.Server)
	}

	viper.Reset()
	write(t, wd, ".truenas-exporter", "SERVER=from-here\n")
	if f := must(t); f.Server != "from-here" {
		t.Errorf("server = %q, want the working directory's over $HOME's", f.Server)
	}
}

// A settings file that is named is the only one read, and one that cannot be read is an error: a run told where its settings are must not come
// up with someone else's, or with none.
func TestANamedSettingsFile(t *testing.T) {
	home, _ := alone(t)
	write(t, home, ".truenas-exporter", "SERVER=from-home\n")
	named := write(t, home, "other.env", "USERNAME=from-named\n")

	f := must(t, "--config", named)
	if f.Username != "from-named" || f.Server != "" {
		t.Errorf("with --config = %+v, want the named file and nothing of $HOME's", f)
	}

	viper.Reset()
	t.Setenv(configEnv, "~/other.env")
	if f := must(t); f.Username != "from-named" {
		t.Errorf("with %s=~/other.env the user = %q, want the named file's", configEnv, f.Username)
	}

	viper.Reset()
	if _, err := load(t, "--config", filepath.Join(home, "missing.env")); err == nil || !strings.Contains(err.Error(), "missing.env") {
		t.Errorf("a named file that is not there = %v, want an error naming it", err)
	}
}

// The NAS, the user and the key are all needed, and whichever is missing is a mistake that is said, naming the variable to set and never the key.
// So is saying two ways to trust the NAS's certificate, since only one of them would be used.
func TestConnection(t *testing.T) {
	t.Parallel()

	whole := FlagData{Server: testServer, Username: testUser, APIKey: testKey, CertificateFile: "/etc/nas.pem", DatasetDepth: 1, ClockCorrection: 250 * time.Millisecond}

	got, err := whole.connection()
	if err != nil {
		t.Fatal(err)
	}
	if want := (connection{server: testServer, username: testUser, key: testKey, certFile: "/etc/nas.pem", clockCorrection: 250 * time.Millisecond}); got != want {
		t.Errorf("connection = %+v, want %+v", got, want)
	}

	for _, one := range []FlagData{{CAFile: "/etc/authority.pem"}, {SkipCertificateCheck: true}, {SkipCertificateCheck: true, SendKeyUnchecked: true}, {}} {
		one.Server, one.Username, one.APIKey = testServer, testUser, testKey

		got, err := one.connection()
		if err != nil || got.caFile != one.CAFile || got.skipCheck != one.SkipCertificateCheck || got.sendUnchecked != one.SendKeyUnchecked || got.certFile != "" {
			t.Errorf("connection of %+v = %+v, %v; want the one way it names of trusting the certificate, or none", one, got, err)
		}
	}

	for name, c := range map[string]struct {
		change func(*FlagData)
		names  string
	}{
		"no NAS":                            {func(f *FlagData) { f.Server = "" }, "TRUENAS_EXPORTER_SERVER"},
		"no user":                           {func(f *FlagData) { f.Username = "" }, "TRUENAS_EXPORTER_USERNAME"},
		"no key":                            {func(f *FlagData) { f.APIKey = "" }, "TRUENAS_EXPORTER_API_KEY"},
		"a depth below nothing":             {func(f *FlagData) { f.DatasetDepth = -1 }, "TRUENAS_EXPORTER_DATASET_DEPTH"},
		"a certificate and an authority":    {func(f *FlagData) { f.CAFile = "/etc/authority.pem" }, "TRUENAS_EXPORTER_CA_FILE"},
		"a certificate and no check at all": {func(f *FlagData) { f.SkipCertificateCheck = true }, "TRUENAS_EXPORTER_CERTIFICATE_FILE"},
		"the key sent unchecked, checked":   {func(f *FlagData) { f.SendKeyUnchecked = true }, "TRUENAS_EXPORTER_SKIP_CERTIFICATE_CHECK"},
	} {
		f := whole
		c.change(&f)

		_, err := f.connection()
		if err == nil || !strings.Contains(err.Error(), c.names) {
			t.Errorf("%s: connection = %v, want an error naming %s", name, err, c.names)
		}
		if err != nil && strings.Contains(err.Error(), testKey) {
			t.Errorf("%s: the error holds the key: %v", name, err)
		}
	}
}

// --metrics names the parts of the NAS to ask about, every one when it names none, and --skip takes some away from them, in any case and with
// spaces about the commas.
func TestSkipped(t *testing.T) {
	t.Parallel()

	allBut := func(kept ...poll.Area) []poll.Area {
		return slices.DeleteFunc(slices.Clone(poll.Areas), func(a poll.Area) bool { return slices.Contains(kept, a) })
	}

	for _, c := range []struct {
		metrics, skip string
		want          []poll.Area
	}{
		{"", "", nil},
		{"", "apps, VMS", []poll.Area{poll.AreaApps, poll.AreaVMs}},
		{"pools,disks", "", allBut(poll.AreaPools, poll.AreaDisks)},
		{"pools, Disks", "disks", allBut(poll.AreaPools)},
		{",", " , ", nil},
	} {
		f := FlagData{Metrics: c.metrics, Skip: c.skip}

		got, err := f.skipped()
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("--metrics %q --skip %q left out %v (%v), want %v", c.metrics, c.skip, got, err, c.want)
		}
	}
}

// A word that names no part of the NAS is refused, naming the setting it was in and the parts there are. The NAS's own state is not one, since it
// is always asked, and a part is named as the page names it: apps, not app.
func TestSkippedRefusesAWordThatNamesNoPart(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ metrics, skip, names, word string }{
		{"", "apps,smart", "TRUENAS_EXPORTER_SKIP", `"smart"`},
		{"system", "", "TRUENAS_EXPORTER_METRICS", `"system"`},
		{"app,disks", "", "TRUENAS_EXPORTER_METRICS", `"app"`},
	} {
		f := FlagData{Metrics: c.metrics, Skip: c.skip}

		if _, err := f.skipped(); err == nil || !strings.Contains(err.Error(), c.names) || !strings.Contains(err.Error(), c.word) || !strings.Contains(err.Error(), "key-expiry") {
			t.Errorf("--metrics %q --skip %q = %v, want an error naming %s, %s and the parts there are", c.metrics, c.skip, err, c.names, c.word)
		}
	}
}

// --intervals changes how often the parts it names are asked about, in any case and with spaces about the commas, and leaves the rest as they are by
// default. The NAS's own state is one of them. A part, or a length of time, that cannot be read is refused, naming the setting.
func TestIntervals(t *testing.T) {
	t.Parallel()

	f := FlagData{Intervals: "snapshots=30m, Apps = 1m,,system=10s"}

	got, err := f.intervals()
	if err != nil {
		t.Fatal(err)
	}

	want := poll.DefaultIntervals()
	want[poll.AreaSnapshots], want[poll.AreaApps], want[poll.AreaSystem] = 30*time.Minute, time.Minute, 10*time.Second
	if !maps.Equal(got, want) {
		t.Errorf("--intervals %q = %v, want %v", f.Intervals, got, want)
	}

	for _, bad := range []string{"smart=1m", "snapshots", "snapshots=soon", "snapshots=0s", "snapshots=-1m", "=1m"} {
		f := FlagData{Intervals: bad}

		if _, err := f.intervals(); err == nil || !strings.Contains(err.Error(), "TRUENAS_EXPORTER_INTERVALS") {
			t.Errorf("--intervals %q = %v, want an error naming the setting", bad, err)
		}
	}
}

// The poller is made with the connection the settings describe. One that cannot be made is said, and an interval that cannot be used is refused
// before anything is connected to.
func TestPoller(t *testing.T) {
	t.Parallel()

	f := FlagData{Server: testServer, Username: testUser, APIKey: testKey}

	var made []connection
	var closed int
	connect := func(c connection) (poll.NAS, func(), error) {
		made = append(made, c)
		return &polltest.NAS{}, func() { closed++ }, nil
	}

	p, closer, err := f.poller(connect)
	if err != nil || p == nil {
		t.Fatalf("poller = %v, %v", p, err)
	}
	if len(made) != 1 || made[0] != (connection{server: testServer, username: testUser, key: testKey}) {
		t.Errorf("connected with %+v, want the settings' NAS, user and key once", made)
	}
	closer()
	if closed != 1 {
		t.Errorf("the connection was closed %d times, want once when the poller is done with", closed)
	}

	_, _, err = f.poller(func(connection) (poll.NAS, func(), error) { return nil, nil, errors.New("no such host") })
	if err == nil || !strings.Contains(err.Error(), "no such host") {
		t.Errorf("a connection that cannot be made = %v, want its error", err)
	}

	f.Intervals = "system=0s"
	if _, _, err := f.poller(connect); err == nil || !strings.Contains(err.Error(), "TRUENAS_EXPORTER_INTERVALS") || len(made) != 1 {
		t.Errorf("an interval of nothing = %v after %d connections; want it refused naming the setting, before connecting", err, len(made))
	}
}
