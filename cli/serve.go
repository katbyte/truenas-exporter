package cli

import (
	"os/signal"
	"syscall"

	"github.com/katbyte/go-kt/prometheus/exporter"
	"github.com/spf13/cobra"
)

func serveCmd(connect connector) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve the metrics page",
		Long: `Asks the NAS for its state on a schedule and serves the last answers at /metrics in the
Prometheus text format, with a health probe at /healthz.

A scrape is answered from memory and never waits on the NAS. Each part of the NAS is asked
about on its own schedule: its own state, pools, alerts, services, jobs, apps, virtual
machines and connected clients every 30 seconds, snapshot counts every 10 minutes, whether
a newer TrueNAS is out every 4 hours, and the rest every 5 minutes, unless --intervals says
otherwise. A NAS that stops answering is reported as truenas_up 0, and what it last said of
each part is served for three of that part's intervals and then left off.

--metrics names the parts of the NAS to ask about, every one unless set, and --skip leaves
some of those out.

The NAS is reached over TLS and no other way: it revokes a key sent in the clear. A NAS as it
comes shows a certificate it made itself, made out to localhost, so say which one is its:
--certificate-file is that certificate, and only a NAS holding its key is spoken to. --ca-file
is for a NAS given a certificate with its own name on it. --skip-certificate-check takes any
certificate, and then a TrueNAS before version 26, which takes a key only as it is, is not
sent the key unless --send-key-unchecked says to hand it to whatever answers.

--dataset-depth says how far below its pool a dataset may be and still be named: the names
further down are often people's. Every dataset and snapshot is counted however deep.

--labels puts labels of your choosing on every reading, hostname=nas say, for when whatever
scrapes the page adds none of its own.

Every setting is also an environment variable: TRUENAS_EXPORTER_SERVER,
TRUENAS_EXPORTER_USERNAME and TRUENAS_EXPORTER_API_KEY, TRUENAS_EXPORTER_CERTIFICATE_FILE,
TRUENAS_EXPORTER_CA_FILE, TRUENAS_EXPORTER_SKIP_CERTIFICATE_CHECK,
TRUENAS_EXPORTER_SEND_KEY_UNCHECKED, TRUENAS_EXPORTER_LISTEN, TRUENAS_EXPORTER_LABELS,
TRUENAS_EXPORTER_DATASET_DEPTH, TRUENAS_EXPORTER_METRICS, TRUENAS_EXPORTER_SKIP,
TRUENAS_EXPORTER_INTERVALS and TRUENAS_EXPORTER_CLOCK_CORRECTION.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

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

			// one signal stops both the polling and the serving
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			return exporter.Serve(ctx, f.Listen, p, page)
		},
	}
}
