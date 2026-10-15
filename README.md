# truenas-exporter - TrueNAS health as a Prometheus metrics page

[![GitHub release](https://img.shields.io/github/v/release/katbyte/truenas-exporter?color=blueviolet)](https://github.com/katbyte/truenas-exporter/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/katbyte/truenas-exporter?color=00ADD8)](https://github.com/katbyte/truenas-exporter/blob/main/go.mod)
[![License](https://img.shields.io/github/license/katbyte/truenas-exporter?color=blue)](https://github.com/katbyte/truenas-exporter/blob/main/LICENSE)
![build](https://github.com/katbyte/truenas-exporter/actions/workflows/build.yaml/badge.svg)
![tests](https://github.com/katbyte/truenas-exporter/actions/workflows/pr-tests.yaml/badge.svg)
![lint](https://github.com/katbyte/truenas-exporter/actions/workflows/pr-golangci-lint.yaml/badge.svg)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/katbyte/truenas-exporter/badges/coverage.json)](https://github.com/katbyte/truenas-exporter/actions/workflows/coverage.yaml)

Reports how a [TrueNAS](https://www.truenas.com) is doing as a [Prometheus](https://prometheus.io) metrics page.

TrueNAS's own reporting says how busy the machine is. This says whether the storage is well: pool health, errors on each disk, scrubs, space, snapshots, alerts and whether last night's backup ran.

**It only reads, and it names no people.** It runs on another machine and installs nothing on the NAS. Share names, client addresses and alert text never reach the page. Written for TrueNAS 25.10.

## Install

Any one of:

```bash
docker pull ghcr.io/katbyte/truenas-exporter:latest
go install github.com/katbyte/truenas-exporter@latest
brew install katbyte/tap/truenas-exporter
```

## Quick start

1. On the NAS, make a user with the read-only administrator role, and an API key for that user.
2. Save the NAS's certificate. Do this from a machine you trust to be talking to the NAS:

   ```bash
   openssl s_client -connect nas.example.net:443 </dev/null 2>/dev/null | openssl x509 > nas.pem
   ```

3. Check it works, then serve the page:

   ```bash
   export TRUENAS_EXPORTER_SERVER=nas.example.net
   export TRUENAS_EXPORTER_USERNAME=metrics
   export TRUENAS_EXPORTER_API_KEY=3-xxxxxxxx
   export TRUENAS_EXPORTER_CERTIFICATE_FILE=nas.pem

   truenas-exporter print   # ask once and print the page
   truenas-exporter serve   # serve it at /metrics, with a health check at /healthz
   ```

To run it in Docker, see [`docker-compose.yml`](docker-compose.yml): set the NAS's address and user in it and the key in a `.env` ([`.env.example`](.env.example)), and for a NAS that still shows the certificate it came with, put `nas.pem` beside it. [More on Docker](docs/settings.md#docker).

## What it reports

| Area | What you see |
|---|---|
| [The NAS](docs/readings.md#the-nas) | version, uptime, load, updates, network links, certificate expiry |
| [Pools](docs/readings.md#pools) | health, status, space, scrubs and resilvers |
| [Disks](docs/readings.md#disks) | status and errors in the pool, size, temperature |
| [Datasets](docs/readings.md#datasets-and-snapshots) | space used and free, quotas, snapshot counts |
| [Alerts](docs/readings.md#alerts) | counts by level and kind |
| [Services, apps and virtual machines](docs/readings.md#services-jobs-apps-and-virtual-machines) | which are running |
| [Shares](docs/readings.md#shares) | counts by protocol, and connected clients |
| [Scheduled tasks](docs/readings.md#scheduled-tasks) | when each last ran, and whether it failed |

Every reading is listed in [docs/readings.md](docs/readings.md).

Datasets are named one level below the pool unless you ask for more, since deeper names are often people's. Every disk reading carries the disk's serial number, so it lines up with readings from other exporters.

Left to others:

- **Processor, memory, network and disk activity.** TrueNAS's own reporting has them.
- **SMART.** The API has none. Run [smartctl_exporter](https://github.com/prometheus-community/smartctl_exporter) on the NAS.
- **Enclosures and slots.** The API only knows TrueNAS's own hardware.

## Settings

Each setting is a flag, an environment variable or a line in a settings file. [docs/settings.md](docs/settings.md) has the detail.

| Variable | Flag | What it is |
|---|---|---|
| `TRUENAS_EXPORTER_SERVER` | `--server` | the NAS's name or address |
| `TRUENAS_EXPORTER_USERNAME` | `--username` | the user the key belongs to |
| `TRUENAS_EXPORTER_API_KEY` | `--api-key` | the API key |
| `TRUENAS_EXPORTER_CERTIFICATE_FILE` | `--certificate-file` | the NAS's own certificate |
| `TRUENAS_EXPORTER_CA_FILE` | `--ca-file` | an authority to trust for the NAS's certificate |
| `TRUENAS_EXPORTER_SKIP_CERTIFICATE_CHECK` | `--skip-certificate-check` | accept any certificate |
| `TRUENAS_EXPORTER_SEND_KEY_UNCHECKED` | `--send-key-unchecked` | send the key without checking the certificate |
| `TRUENAS_EXPORTER_LISTEN` | `--listen` | where the page is served, default `:9089` |
| `TRUENAS_EXPORTER_LABELS` | `--labels` | labels for every reading, as `name=value,name=value` |
| `TRUENAS_EXPORTER_INTERVALS` | `--intervals` | how often to ask about a part, like `snapshots=30m` |
| `TRUENAS_EXPORTER_CLOCK_CORRECTION` | `--clock-correction` | added to the NAS's clock, default `500ms` |
| `TRUENAS_EXPORTER_DATASET_DEPTH` | `--dataset-depth` | how deep datasets are named, default `1` |
| `TRUENAS_EXPORTER_METRICS` | `--metrics` | the parts to ask about, like `pools,disks`; default all |
| `TRUENAS_EXPORTER_SKIP` | `--skip` | parts to leave out, like `apps,vms` |
| `TRUENAS_EXPORTER_LOG` | | log level: `WARN` (default), `INFO` or `DEBUG` |
| `TRUENAS_EXPORTER_CONFIG` | `--config` | the settings file to read |

### The certificate

A new TrueNAS shows a certificate made out to `localhost`, so it cannot be checked by name. Choose one:

- **`--certificate-file`** trusts exactly the NAS's own certificate. Use it for a NAS as it comes.
- **`--ca-file`** is for a NAS you gave a certificate with its real name on it.
- **`--skip-certificate-check`** accepts any certificate, so nothing proves it is your NAS.

An address starting `http://` or `ws://` is refused. TrueNAS revokes a key sent without encryption.

## Scraping

Grafana Alloy:

```alloy
prometheus.scrape "truenas" {
  targets         = [{"__address__" = "127.0.0.1:9089"}]
  job_name        = "truenas"
  scrape_interval = "30s"
  forward_to      = [prometheus.remote_write.default.receiver]
}
```

Prometheus or vmagent:

```yaml
scrape_configs:
  - job_name: truenas
    scrape_interval: 30s
    static_configs:
      - targets: ['127.0.0.1:9089']
```

A scrape is answered from memory and never waits on the NAS. If the NAS stops answering, `truenas_up` goes to 0.

A few alerts worth having, with more in [docs/queries.md](docs/queries.md):

```promql
truenas_up == 0                        # the NAS is not answering
truenas_pool_healthy == 0              # a pool is not healthy
truenas_task_last_run_failed == 1      # a scheduled task failed
```

## How this is built

The code is written with Claude Code. I review it, then prune or rework it until it has the shape I want. I set the design and the scope, and I decide what stays.

What CI holds it to:

- **Tests.** Unit tests run under the race detector. The whole page is saved and compared. A test fails if the program calls anything but its list of read-only methods.
- **Live tests.** CI installs a real TrueNAS in a virtual machine and checks the page against what that NAS says. That NAS has a pool, a share of each kind, a virtual machine that is not running, and tasks that ran, never ran and failed. It has no apps, cloud tasks or failing disks.
- **Lint.** golangci-lint, markdownlint, typos, yamllint, shellcheck and actionlint.
- **Security.** CodeQL, govulncheck, Trivy on the built program, and zizmor on the workflows.

## More

- [docs/readings.md](docs/readings.md): every reading and its labels.
- [docs/settings.md](docs/settings.md): the user and key, the certificate, the settings file.
- [docs/queries.md](docs/queries.md): alerts and queries to start from.
- [docs/development.md](docs/development.md): building, the tests and how the code is laid out.
- [CONTRIBUTING.md](CONTRIBUTING.md): sending a change, and how releases are cut.
