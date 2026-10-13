package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// The defaults: what changes by the minute is read as often as a scrape every 30 seconds asks, and the rest every few minutes, since it only
// changes when somebody changes it or a task runs.
const (
	defaultListen       = ":9089"
	defaultFastInterval = 30 * time.Second
	defaultSlowInterval = 5 * time.Minute
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
	FastInterval         time.Duration `mapstructure:"fast-interval"`
	SlowInterval         time.Duration `mapstructure:"slow-interval"`
	DatasetDepth         int           `mapstructure:"dataset-depth"`
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
	"fast-interval":          "TRUENAS_METRICS_FAST_INTERVAL",
	"slow-interval":          "TRUENAS_METRICS_SLOW_INTERVAL",
	"dataset-depth":          "TRUENAS_METRICS_DATASET_DEPTH",
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
	pflags.Duration("fast-interval", defaultFastInterval, "how often pools and their scans, alerts, services, jobs, apps and the NAS's own state are read")
	pflags.Duration("slow-interval", defaultSlowInterval, "how often disks, datasets, tasks, shares, certificates and updates are read")
	pflags.Int("dataset-depth", readings.DefaultDatasetDepth, "how far below its pool a dataset may be and still be named on the page: 0 for each pool's own alone, 1 for those directly under it too")
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

	return connection{server: f.Server, username: f.Username, key: f.APIKey, certFile: f.CertificateFile, caFile: f.CAFile, skipCheck: f.SkipCertificateCheck, sendUnchecked: f.SendKeyUnchecked}, nil
}

// poller is a poller for the NAS the settings name, not yet started, and what closes its connection.
func (f *FlagData) poller(connect connector) (*poll.Poller, func(), error) {
	c, err := f.connection()
	if err != nil {
		return nil, nil, err
	}

	nas, closer, err := connect(c)
	if err != nil {
		return nil, nil, err
	}

	p, err := poll.New(nas, poll.Intervals{Fast: f.FastInterval, Slow: f.SlowInterval})
	if err != nil {
		closer()

		return nil, nil, fmt.Errorf("--fast-interval and --slow-interval (%s, %s): %w", envNames["fast-interval"], envNames["slow-interval"], err)
	}

	// the address and the user are logged and the key never is
	clog.Log.Infof("reporting on the TrueNAS at %s as %s, every %s and every %s", f.Server, f.Username, f.FastInterval, f.SlowInterval)

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
