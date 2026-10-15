package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/pmetrics"
	"github.com/katbyte/truenas-metrics/lib/poll"
	"github.com/katbyte/truenas-metrics/lib/readings"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// toolName is what the program is called, on the page's front and in its help.
const toolName = "truenas-metrics"

// The defaults: where the page is served, and what is added to the NAS's clock, which it gives to the whole second, to put the reading of it in
// the middle of where the time could be.
const (
	defaultListen          = ":9089"
	defaultClockCorrection = 500 * time.Millisecond
)

type FlagData struct {
	Server               string        `mapstructure:"server"`
	Username             string        `mapstructure:"username"`
	APIKey               string        `mapstructure:"api-key"`
	CertificateFile      string        `mapstructure:"certificate-file"`
	CAFile               string        `mapstructure:"ca-file"`
	SkipCertificateCheck bool          `mapstructure:"skip-certificate-check"`
	SendKeyUnchecked     bool          `mapstructure:"send-key-unchecked"`
	Listen               string        `mapstructure:"listen"`
	Labels               string        `mapstructure:"labels"`
	DatasetDepth         int           `mapstructure:"dataset-depth"`
	Metrics              string        `mapstructure:"metrics"`
	Skip                 string        `mapstructure:"skip"`
	Intervals            string        `mapstructure:"intervals"`
	ClockCorrection      time.Duration `mapstructure:"clock-correction"`
}

// envNames binds each flag to its environment variable.
var envNames = map[string]string{
	"server":                 "TRUENAS_METRICS_SERVER",
	"username":               "TRUENAS_METRICS_USERNAME",
	"api-key":                "TRUENAS_METRICS_API_KEY",
	"certificate-file":       "TRUENAS_METRICS_CERTIFICATE_FILE",
	"ca-file":                "TRUENAS_METRICS_CA_FILE",
	"skip-certificate-check": "TRUENAS_METRICS_SKIP_CERTIFICATE_CHECK",
	"send-key-unchecked":     "TRUENAS_METRICS_SEND_KEY_UNCHECKED",
	"listen":                 "TRUENAS_METRICS_LISTEN",
	"labels":                 "TRUENAS_METRICS_LABELS",
	"dataset-depth":          "TRUENAS_METRICS_DATASET_DEPTH",
	"metrics":                "TRUENAS_METRICS_METRICS",
	"skip":                   "TRUENAS_METRICS_SKIP",
	"intervals":              "TRUENAS_METRICS_INTERVALS",
	"clock-correction":       "TRUENAS_METRICS_CLOCK_CORRECTION",
}

// persistent is the flag set configureFlags bound, for the one setting that has to be read before viper is: where the settings file is.
var persistent *pflag.FlagSet

func configureFlags(root *cobra.Command) error {
	pflags := root.PersistentFlags()

	pflags.String("server", "", "the TrueNAS's name or address, e.g. nas.example.net or https://192.168.1.5; it is only ever reached over TLS")
	pflags.String("username", "", "the user the API key was made for")
	pflags.String("api-key", "", "the API key (consider exporting to TRUENAS_METRICS_API_KEY instead)")
	pflags.String("certificate-file", "", "the NAS's own certificate as a PEM file: only a NAS that holds its key is spoken to, whatever name is on it, which is what fits the certificate a TrueNAS comes with")
	pflags.String("ca-file", "", "trust the certificates in this PEM file as well as the system's, for a NAS given a certificate with its own name on it")
	pflags.Bool("skip-certificate-check", false, "take whatever certificate the NAS shows: nothing then says it is the NAS that answers, and a TrueNAS before version 26 is not sent the key unless --send-key-unchecked is given too")
	pflags.Bool("send-key-unchecked", false, "with --skip-certificate-check, send the key to a TrueNAS before version 26 all the same: it takes a key only as it is, so whatever answers at the address is handed it")
	pflags.String("listen", defaultListen, "serve the metrics page on this address")
	pflags.String("labels", "", "labels to put on every reading, as name=value,name=value: hostname=nas say, when whatever scrapes the page adds none of its own")
	pflags.Int("dataset-depth", readings.DefaultDatasetDepth, "how far below its pool a dataset may be and still be named on the page: 0 for each pool's own alone, 1 for those directly under it too")
	pflags.String("metrics", "", "the parts of the NAS to ask about and no others, with commas between them: "+areaNames(poll.Areas)+"; every one unless set, and the NAS's own state always")
	pflags.String("skip", "", "leave these parts of the NAS out of those --metrics names, named as for it: apps,vms say; nothing of them is on the page")
	pflags.String("intervals", "", "how often to ask about some parts of the NAS, as part=duration with commas between them: snapshots=30m,apps=1m say; unset, "+defaultsSaid())
	pflags.Duration("clock-correction", defaultClockCorrection, "added to the time the NAS gives before it is held to this machine's clock: a TrueNAS gives it to the whole second, so half a second centres the reading; 0 for a NAS that gives it finer")
	pflags.String("config", "", "read settings from this file and no other, in place of ./.truenas-metrics or ~/.truenas-metrics (or export TRUENAS_METRICS_CONFIG)")

	persistent = pflags

	for name, env := range envNames {
		if err := viper.BindPFlag(name, pflags.Lookup(name)); err != nil {
			return fmt.Errorf("error binding '%s' flag: %w", name, err)
		}

		if err := viper.BindEnv(name, env); err != nil {
			return fmt.Errorf("error binding '%s' to env '%s' : %w", name, env, err)
		}
	}

	return nil
}

// configEnv names the settings file from the environment, as --config does on the command line.
const configEnv = "TRUENAS_METRICS_CONFIG"

// configPath is the settings file a run was told to read, "" for none: the flag's, then the environment's. A leading ~/ is the home directory, for
// whatever starts the program without a shell to expand it.
func configPath() string {
	path := os.Getenv(configEnv)
	if persistent != nil {
		if fl := persistent.Lookup("config"); fl != nil && fl.Changed {
			path = fl.Value.String()
		}
	}

	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, rest)
		}
	}

	return path
}

// loadConfig reads the settings file, once the flags are parsed: the one --config or TRUENAS_METRICS_CONFIG names and no other, or else the first
// .truenas-metrics found in the working directory and then in $HOME. A file that was named and cannot be read is an error; one merely absent from
// the usual places is not.
func loadConfig() error {
	viper.SetConfigType("env")
	if path := configPath(); path != "" {
		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("reading the settings file %s (--config / %s): %w", path, configEnv, err)
		}
	} else {
		viper.SetConfigName("." + toolName)
		// viper reads the first file it finds, so the working directory comes first
		viper.AddConfigPath(".")
		if home, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(home)
		}

		if err := viper.ReadInConfig(); err != nil {
			if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); !ok {
				clog.Log.Errorf("Error reading config file: %v", err)
			}
		}
	}

	// a settings file spells a setting the way the environment does, less the prefix (API_KEY for --api-key), and viper only matches a key spelled
	// as the flag is. Carried across as defaults, they still lose to a flag or the environment.
	for name := range envNames {
		if alt := strings.ReplaceAll(name, "-", "_"); alt != name && viper.InConfig(alt) {
			viper.SetDefault(name, viper.Get(alt))
		}
	}

	return nil
}

// GetFlags returns the fully populated FlagData. It is read from viper rather than from the flags themselves, since viper is what lays the
// environment and the settings file under the command line.
func GetFlags() *FlagData {
	var f FlagData
	if err := viper.Unmarshal(&f); err != nil {
		clog.Log.Fatalf("failed to unmarshal configuration: %v", err)
	}

	return &f
}

// connection is what it takes to reach a NAS and log in to it.
type connection struct {
	server   string
	username string
	key      string
	// certFile is the NAS's own certificate, caFile a file of certificates to trust beside the system's own, and skipCheck takes any certificate
	// at all. At most one of the three is set.
	certFile  string
	caFile    string
	skipCheck bool
	// sendUnchecked has the key sent to a NAS whose certificate was not checked, which only an older TrueNAS asks for.
	sendUnchecked bool
	// clockCorrection is added to the time the NAS gives, as nas.Options says.
	clockCorrection time.Duration
}

// connector makes what asks the NAS its questions, and what closes the connection it asks them over. Nothing is sent until the first question.
type connector func(c connection) (nas poll.NAS, closer func(), err error)

// connection is how the settings say to reach the NAS. A setting left out is a mistake said now, naming the variable to set, rather than a page
// that only ever says the NAS is down.
func (f *FlagData) connection() (connection, error) {
	for _, need := range []struct{ flag, value, what string }{{"server", f.Server, "no NAS to report on"}, {"username", f.Username, "no user to log in as"}, {"api-key", f.APIKey, "no API key to log in with"}} {
		if need.value == "" {
			return connection{}, fmt.Errorf("%s: set --%s (%s)", need.what, need.flag, envNames[need.flag])
		}
	}

	var trusting int
	for _, set := range []bool{f.CertificateFile != "", f.CAFile != "", f.SkipCertificateCheck} {
		if set {
			trusting++
		}
	}

	if trusting > 1 {
		return connection{}, fmt.Errorf("--certificate-file, --ca-file and --skip-certificate-check (%s, %s, %s) are three ways of doing one thing: set one of them", envNames["certificate-file"], envNames["ca-file"], envNames["skip-certificate-check"])
	}

	if f.SendKeyUnchecked && !f.SkipCertificateCheck {
		return connection{}, fmt.Errorf("--send-key-unchecked (%s) only says what to do when the certificate is not checked: it goes with --skip-certificate-check (%s)", envNames["send-key-unchecked"], envNames["skip-certificate-check"])
	}

	if f.DatasetDepth < 0 {
		return connection{}, fmt.Errorf("--dataset-depth (%s) cannot be less than 0, which is each pool's own dataset: %d", envNames["dataset-depth"], f.DatasetDepth)
	}

	return connection{server: f.Server, username: f.Username, key: f.APIKey, certFile: f.CertificateFile, caFile: f.CAFile, skipCheck: f.SkipCertificateCheck, sendUnchecked: f.SendKeyUnchecked, clockCorrection: f.ClockCorrection}, nil
}

// skipped is the parts of the NAS the settings leave unasked: those --metrics does not name, when it names any, and those --skip names. A word that
// names no part is a mistake said now, since a page quietly missing what was asked for reads as a NAS with none of it.
func (f *FlagData) skipped() ([]poll.Area, error) {
	metrics, err := areas("metrics", f.Metrics)
	if err != nil {
		return nil, err
	}

	skip, err := areas("skip", f.Skip)
	if err != nil {
		return nil, err
	}

	if len(metrics) == 0 {
		metrics = poll.Areas
	}

	var out []poll.Area
	for _, area := range poll.Areas {
		if !slices.Contains(metrics, area) || slices.Contains(skip, area) {
			out = append(out, area)
		}
	}

	return out, nil
}

// areas is the parts of the NAS one setting names.
func areas(flag, list string) ([]poll.Area, error) {
	var out []poll.Area
	for word := range strings.SplitSeq(list, ",") {
		area := poll.Area(strings.ToLower(strings.TrimSpace(word)))
		switch {
		case area == "":
		case slices.Contains(poll.Areas, area):
			out = append(out, area)
		default:
			return nil, fmt.Errorf("--%s (%s) takes %s, with commas between them, and not %q", flag, envNames[flag], areaNames(poll.Areas), strings.TrimSpace(word))
		}
	}

	return out, nil
}

// areaNames is areas as a list to read.
func areaNames(areas []poll.Area) string {
	names := make([]string, len(areas))
	for i, area := range areas {
		names[i] = string(area)
	}

	return strings.Join(names, ", ")
}

// intervals is how often the settings say to ask about each part of the NAS: as often as it is by default, but for the parts --intervals names. A
// part or a length of time that cannot be read is a mistake said now.
func (f *FlagData) intervals() (poll.Intervals, error) {
	every := poll.DefaultIntervals()
	for pair := range strings.SplitSeq(f.Intervals, ",") {
		if strings.TrimSpace(pair) == "" {
			continue
		}

		name, length, ok := strings.Cut(pair, "=")
		area := poll.Area(strings.ToLower(strings.TrimSpace(name)))
		if _, known := every[area]; !ok || !known {
			return nil, fmt.Errorf("--intervals (%s) takes part=duration with commas between them, a part being %s, and not %q", envNames["intervals"], areaNames(poll.Every), strings.TrimSpace(pair))
		}

		d, err := time.ParseDuration(strings.TrimSpace(length))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("--intervals (%s) says how often %s is asked about, as 30s or 10m say, and not %q", envNames["intervals"], area, strings.TrimSpace(length))
		}

		every[area] = d
	}

	return every, nil
}

// defaultsSaid is how often each part of the NAS is asked about by default, as the help says it.
func defaultsSaid() string {
	every := poll.DefaultIntervals()

	var lengths []time.Duration
	for _, area := range poll.Every {
		if !slices.Contains(lengths, every[area]) {
			lengths = append(lengths, every[area])
		}
	}

	said := make([]string, 0, len(lengths))
	for _, length := range lengths {
		areas := slices.DeleteFunc(slices.Clone(poll.Every), func(a poll.Area) bool { return every[a] != length })
		said = append(said, areaNames(areas)+" every "+short(length))
	}

	return strings.Join(said, "; ")
}

// short is a length of time as a person writes it: 5m, not 5m0s.
func short(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}

	return s
}

// poller is a poller for the NAS the settings name, not yet started, and what closes its connection.
func (f *FlagData) poller(connect connector) (*poll.Poller, func(), error) {
	c, err := f.connection()
	if err != nil {
		return nil, nil, err
	}

	skip, err := f.skipped()
	if err != nil {
		return nil, nil, err
	}

	every, err := f.intervals()
	if err != nil {
		return nil, nil, err
	}

	nas, closer, err := connect(c)
	if err != nil {
		return nil, nil, err
	}

	p, err := poll.New(nas, every, skip...)
	if err != nil {
		closer()

		return nil, nil, fmt.Errorf("--intervals (%s): %w", envNames["intervals"], err)
	}

	// the address and the user are logged and the key never is
	clog.Log.Infof("reporting on the TrueNAS at %s as %s", f.Server, f.Username)
	if f.Intervals != "" {
		clog.Log.Infof("asking about %s as often as --intervals says", f.Intervals)
	}
	if len(skip) > 0 {
		clog.Log.Infof("not asking about %s", areaNames(skip))
	}

	switch {
	case f.SendKeyUnchecked:
		clog.Log.Warn("the NAS's certificate is not being checked and the key is sent all the same, so whatever answers at its address is given the key: --certificate-file checks it")
	case f.SkipCertificateCheck:
		clog.Log.Warn("the NAS's certificate is not being checked, so nothing says it is the NAS that answers: --certificate-file checks it")
	}

	return p, closer, nil
}

// page is the metrics page of this poller's readings, with the labels the settings ask for on every one. A label that cannot be used is said here,
// before the NAS is asked anything: one written wrongly, or one named as a reading's own label is, pool say.
func (f *FlagData) page(p *poll.Poller) (*pmetrics.Page, error) {
	labels, err := pmetrics.ParseLabels(f.Labels)
	if err != nil {
		return nil, fmt.Errorf("--labels (%s): %w", envNames["labels"], err)
	}

	page, err := pmetrics.New(pmetrics.Options{Name: toolName, Namespace: readings.Namespace, Labels: labels}, readings.New(p, readings.Options{DatasetDepth: f.DatasetDepth}))
	if err != nil {
		return nil, fmt.Errorf("--labels (%s): %w", envNames["labels"], err)
	}

	return page, nil
}
