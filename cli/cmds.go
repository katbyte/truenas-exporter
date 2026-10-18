// Package cli implements the truenas-exporter command line interface: the cobra commands, flag and config handling, and the server that serves the
// metrics page.
package cli

import (
	"fmt"
	"strings"

	"github.com/katbyte/go-kt/clog"
	"github.com/katbyte/go-kt/prometheus/exporter"
	"github.com/katbyte/go-kt/version"
	"github.com/spf13/cobra"
)

// versionSaid is the version the program gives of itself: a release's tag and the commit it was built from, or for a local build, how far past
// the tag it is and at which commit, which names the commit already.
func versionSaid() string {
	if version.GitCommit == "" || strings.Contains(version.Version, version.GitCommit) {
		return version.Version
	}

	return version.Version + " (" + version.GitCommit + ")"
}

// started logs which version is running, first thing, so a log says what wrote it.
func started() {
	clog.Log.Infof("starting %s %s", toolName, versionSaid())
}

func Make() (*cobra.Command, error) {
	return makeWith(connect)
}

// makeWith is Make with the NAS's client made by connect, so a test can stand in for a NAS.
func makeWith(connect connector) (*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "truenas-exporter [command]",
		Short: "truenas-exporter reports what a TrueNAS says about itself as a Prometheus metrics page",
		Long: `Reports what a TrueNAS says about itself as a Prometheus metrics page: the health of its
pools and the errors against each disk, scrubs and resilvers, space by dataset, snapshots,
alerts, how its scheduled tasks last went, services, apps and virtual machines. It only
ever asks the NAS questions, over its API.
Complete documentation is available at https://github.com/katbyte/truenas-exporter`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Run \"truenas-exporter help\" for more information about available truenas-exporter commands.")
			return nil
		},
	}

	root.AddCommand(&cobra.Command{
		Use:           "version",
		Short:         "Print the version number of truenas-exporter",
		Long:          `Print the version number of truenas-exporter`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "truenas-exporter "+versionSaid())
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "print",
		Short: "Ask the NAS once and print the metrics page",
		Long: `Asks the NAS every question once and prints the page a scrape would read, without serving
anything: a check that the address, the user and the key work, and a look at what would be
reported.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			started()

			f := GetFlags()
			p, closer, err := f.poller(connect)
			if err != nil {
				return err
			}
			defer closer()

			page, err := f.page(p)
			if err != nil {
				return err
			}

			// the page is printed either way, with what a NAS that did not answer looks like on it; why each part failed is in the log
			return exporter.Print(cmd.Context(), cmd.OutOrStdout(), p, page)
		},
	})

	root.AddCommand(serveCmd(connect))

	if err := configureFlags(root); err != nil {
		return nil, fmt.Errorf("unable to configure flags: %w", err)
	}
	// the settings file is read once the flags are parsed, since a flag can name it; before any command's own checks, which read what it holds
	root.PersistentPreRunE = func(*cobra.Command, []string) error { return loadConfig() }

	return root, nil
}
