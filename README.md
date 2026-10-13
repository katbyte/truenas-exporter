# truenas-metrics - what a TrueNAS says about itself, as a Prometheus metrics page

[![GitHub release](https://img.shields.io/github/v/release/katbyte/truenas-metrics?color=blueviolet)](https://github.com/katbyte/truenas-metrics/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/katbyte/truenas-metrics?color=00ADD8)](https://github.com/katbyte/truenas-metrics/blob/main/go.mod)
[![License](https://img.shields.io/github/license/katbyte/truenas-metrics?color=blue)](https://github.com/katbyte/truenas-metrics/blob/main/LICENSE)
![build](https://github.com/katbyte/truenas-metrics/actions/workflows/build.yaml/badge.svg)
![tests](https://github.com/katbyte/truenas-metrics/actions/workflows/pr-tests.yaml/badge.svg)
![lint](https://github.com/katbyte/truenas-metrics/actions/workflows/pr-golangci-lint.yaml/badge.svg)
[![coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/katbyte/truenas-metrics/badges/coverage.json)](https://github.com/katbyte/truenas-metrics/actions/workflows/coverage.yaml)

One small program that asks a [TrueNAS](https://www.truenas.com) how it is doing and reports the answers as a [Prometheus](https://prometheus.io) metrics page: the health of its pools and the errors against each disk, scrubs and resilvers, space by dataset, snapshots, alerts, how its scheduled tasks last went, services, apps and virtual machines.

It only ever asks the NAS questions, over its API, and runs somewhere else: nothing is installed on the NAS. Written for TrueNAS 25.10, against answers saved from a 25.10.7.

## What it is for

TrueNAS can send its own readings to a metrics store, and those cover the machine: processor, memory, network and what each disk is reading and writing. They say nothing of the storage itself. This is the other half: whether a pool is healthy, which disk the errors are against, when the last scrub ran and what it found, whether last night's replication worked.

What it leaves to others:

- **What each disk is doing by the second**, the processor, memory and network: TrueNAS's own reporting has them.
- **SMART**: the API has nothing of it. Run [smartctl_exporter](https://github.com/prometheus-community/smartctl_exporter) on the NAS.
- **Enclosures, slots and the SAS links to them**: the API knows its maker's own enclosures and little else.

Every reading of a disk carries the disk's serial number, so readings of the same disk from another exporter join on it.

## What is on the page

Every reading starts `truenas_`. Statuses, states and levels are written as TrueNAS writes them: `ONLINE`, `RUNNING`, `CRITICAL`.

### The asking itself

| Reading | Labels | What it says |
|---|---|---|
| `truenas_up` | | 1 when the NAS answered every question the last time it was asked the ones that change by the minute, 0 when not |
| `truenas_poll_duration_seconds` | `poll` | how long the last round of questions took: `fast` or `slow` |
| `truenas_poll_last_success_timestamp_seconds` | `poll` | when a round was last answered in full |
| `truenas_poll_failures_total` | `poll` | rounds that have failed since truenas-metrics started |
| `truenas_metrics_build_info` | `version`, `commit`, `goversion` | always 1; the version of truenas-metrics that wrote the page |

### The NAS

| Reading | Labels | What it says |
|---|---|---|
| `truenas_system_info` | `version`, `product`, `manufacturer`, `cpu` | always 1; the version of TrueNAS and the machine it runs on |
| `truenas_system_state` | `state` | 1 for the state the NAS is in and 0 for the others: `BOOTING`, `READY`, `SHUTTING_DOWN` |
| `truenas_system_boot_timestamp_seconds` | | when it last started |
| `truenas_system_uptime_seconds` | | how long it had been running when last asked |
| `truenas_system_memory_bytes` | | how much memory it has |
| `truenas_system_cpu_cores`, `truenas_system_cpu_physical_cores` | | processor cores, with and without threads counted |
| `truenas_system_load_average` | `window` | its load average over `1m`, `5m` and `15m` |
| `truenas_system_ecc_memory` | | 1 when its memory corrects its own errors |
| `truenas_system_clock_offset_seconds` | | its clock less the clock of the machine running truenas-metrics, to within a second |
| `truenas_update_available` | | 1 when it says a newer TrueNAS is available |
| `truenas_update_check_failed` | | 1 when it could not find out |
| `truenas_interface_link_up` | `interface`, `type` | 1 when the network interface has a link |
| `truenas_interface_mtu_bytes` | `interface` | the largest packet the interface sends |
| `truenas_certificate_expiry_timestamp_seconds` | `certificate` | when each certificate it holds runs out |
| `truenas_directory_service_healthy` | `type`, `status` | 1 when the directory it is joined to is healthy; only on a NAS joined to one |
| `truenas_api_key_expiry_timestamp_seconds` | | when the key truenas-metrics uses runs out; only for a key that does |

### Pools

The boot pool is one of them, as `boot-pool`.

| Reading | Labels | What it says |
|---|---|---|
| `truenas_pool_healthy` | `pool` | 1 when the NAS calls the pool healthy |
| `truenas_pool_warning` | `pool` | 1 when it has a warning about the pool |
| `truenas_pool_status` | `pool`, `status` | always 1; the pool's status: `ONLINE`, `DEGRADED`, `FAULTED`, `OFFLINE`, `UNAVAIL`, `REMOVED` |
| `truenas_pool_size_bytes`, `truenas_pool_allocated_bytes`, `truenas_pool_free_bytes` | `pool` | the pool's size and how much is and is not written to, parity and mirrors included |
| `truenas_pool_freeing_bytes` | `pool` | how much is still to be given back after something was deleted |
| `truenas_pool_fragmentation_ratio` | `pool` | how broken up its free space is, from 0 to 1 |
| `truenas_pool_scan_running` | `pool`, `function` | 1 while a scan is under way, for both `SCRUB` and `RESILVER` |
| `truenas_pool_scan_progress_ratio` | `pool`, `function` | how far the last scan got, from 0 to 1 |
| `truenas_pool_scan_errors` | `pool`, `function` | errors the last scan found |
| `truenas_pool_scan_start_timestamp_seconds`, `truenas_pool_scan_end_timestamp_seconds` | `pool`, `function` | when the last scan started and ended; the end is left off while one runs |
| `truenas_pool_scan_to_examine_bytes`, `truenas_pool_scan_examined_bytes`, `truenas_pool_scan_issued_bytes` | `pool`, `function` | how much the scan had to look at, has looked at, and has read and checked |
| `truenas_pool_scan_remaining_seconds` | `pool`, `function` | how long the NAS expects a running scan to go on |
| `truenas_pool_scan_paused` | `pool`, `function` | 1 when the scan is paused |
| `truenas_pool_vdev_info` | `pool`, `class`, `vdev`, `type` | always 1; the kind of each top-level vdev: `RAIDZ2`, `MIRROR`, `DISK` |
| `truenas_pool_vdev_status` | `pool`, `class`, `vdev`, `status` | always 1; the status of each top-level vdev |
| `truenas_pool_vdev_size_bytes`, `truenas_pool_vdev_allocated_bytes`, `truenas_pool_vdev_fragmentation_ratio` | `pool`, `class`, `vdev` | the size, use and fragmentation of each vdev that holds data |
| `truenas_pool_vdev_read_errors`, `truenas_pool_vdev_write_errors`, `truenas_pool_vdev_checksum_errors` | `pool`, `class`, `vdev` | what has gone wrong with each vdev since the counts were last cleared |
| `truenas_pool_device_status` | `pool`, `class`, `vdev`, `disk`, `serial`, `status` | always 1; the status of each disk in a pool |
| `truenas_pool_device_read_errors`, `truenas_pool_device_write_errors`, `truenas_pool_device_checksum_errors` | `pool`, `class`, `vdev`, `disk`, `serial` | what has gone wrong with each disk in a pool since the counts were last cleared |

`class` is the part of the pool a vdev belongs to: `data`, `log`, `cache`, `spare`, `special` or `dedup`. A vdev of one disk goes by that disk's name. The pool remembers one scan, so the scan readings are of the last scrub or the last resilver, whichever was later.

### Disks

| Reading | Labels | What it says |
|---|---|---|
| `truenas_disk_info` | `disk`, `serial`, `wwn`, `model`, `type`, `bus`, `pool`, `enclosure`, `slot` | always 1; what is known of each disk, in a pool or not |
| `truenas_disk_size_bytes` | `disk`, `serial` | the size of each disk |
| `truenas_disk_temperature_celsius` | `disk`, `serial` | the temperature of each disk that gives one |

`disk` is the name the NAS has for the disk, `sda` say, which can change when it restarts; `serial` is what stays. `enclosure` and `slot` are empty wherever the NAS does not know the enclosure.

### Datasets and snapshots

| Reading | Labels | What it says |
|---|---|---|
| `truenas_datasets` | `pool` | how many datasets the pool has, however deep |
| `truenas_snapshots` | `pool` | how many snapshots the pool has, of every dataset however deep |
| `truenas_dataset_used_bytes` | `dataset`, `pool` | how much the dataset takes up, what is under it and its snapshots included |
| `truenas_dataset_available_bytes` | `dataset`, `pool` | how much it may still grow by |
| `truenas_dataset_used_by_dataset_bytes` | `dataset`, `pool` | how much its own data takes up |
| `truenas_dataset_used_by_snapshots_bytes` | `dataset`, `pool` | how much its snapshots alone hold on to |
| `truenas_dataset_quota_bytes`, `truenas_dataset_refquota_bytes` | `dataset`, `pool` | its quotas; only where one is set |
| `truenas_dataset_compression_ratio` | `dataset`, `pool` | how many times smaller it is stored than it is |
| `truenas_dataset_snapshots` | `dataset`, `pool` | how many snapshots it has of itself |

Only each pool's own dataset and the datasets directly under it are named, since the names further down are often people's. `TRUENAS_METRICS_DATASET_DEPTH` names them deeper, or with `0` not at all below the pool. The two counts take in every dataset whatever the depth.

### Alerts

| Reading | Labels | What it says |
|---|---|---|
| `truenas_alerts` | `level` | alerts nobody has dismissed, by level: `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`, `ALERT`, `EMERGENCY` |
| `truenas_alerts_by_class` | `level`, `class` | the same by kind as well, `RsyncFailed` say |
| `truenas_alerts_dismissed` | | alerts somebody has dismissed that the NAS still holds |

What an alert says is not on the page: it names paths and hosts.

### Services, jobs, apps and virtual machines

| Reading | Labels | What it says |
|---|---|---|
| `truenas_service_running` | `service` | 1 when the service is running: `cifs`, `nfs`, `ssh` and the rest |
| `truenas_service_enabled` | `service` | 1 when the service starts with the NAS |
| `truenas_jobs` | `state` | jobs that are not over, by state: `RUNNING`, `WAITING` |
| `truenas_jobs_running` | `method` | jobs running, by what they do: `cloudsync.sync` say |
| `truenas_apps_service_status` | `status` | always 1; the state of what runs apps, `UNCONFIGURED` where apps were never set up |
| `truenas_apps` | `state` | installed apps, by state: `RUNNING`, `DEPLOYING`, `STOPPING`, `STOPPED`, `CRASHED` |
| `truenas_app_state` | `app`, `state` | always 1; the state of each app |
| `truenas_app_update_available` | `app` | 1 when there is a newer version of the app |
| `truenas_vms` | `state` | virtual machines, by state: `RUNNING`, `STOPPED`, `SUSPENDED` |
| `truenas_vm_state` | `vm`, `state` | always 1; the state of each virtual machine |

### Shares

| Reading | Labels | What it says |
|---|---|---|
| `truenas_shares` | `protocol`, `state` | shares over `smb`, `nfs` and `iscsi` that are `enabled`, `disabled`, or `locked` with their dataset |
| `truenas_share_clients` | `protocol` | clients connected over `nfs` and `iscsi`; the NAS gives no count for SMB |

No share's name or path is on the page, nor who is connected.

### Scheduled tasks

`kind` is `snapshot`, `replication`, `cloud_sync`, `cloud_backup`, `rsync` or `scrub`.

| Reading | Labels | What it says |
|---|---|---|
| `truenas_tasks` | `kind` | how many tasks there are |
| `truenas_tasks_enabled` | `kind` | how many are switched on |
| `truenas_tasks_running` | `kind` | how many are running now |
| `truenas_tasks_failed` | `kind` | how many failed the last time they ran |
| `truenas_task_enabled` | `kind`, `id`, `name` | 1 when the task is switched on |
| `truenas_task_running` | `kind`, `id`, `name` | 1 while the task runs |
| `truenas_task_progress_ratio` | `kind`, `id`, `name` | how far a running task says it has got, from 0 to 1 |
| `truenas_task_last_run_failed` | `kind`, `id`, `name` | 1 when the task's last run failed |
| `truenas_task_last_run_timestamp_seconds` | `kind`, `id`, `name` | when its last run ended |
| `truenas_task_last_run_duration_seconds` | `kind`, `id`, `name` | how long its last run took, for cloud sync, cloud backup and rsync |

`id` is the task's number in TrueNAS, and `name` what whoever made it called it; a snapshot task goes by its dataset and a scrub by its pool. A run stopped by hand is over and did not fail. A snapshot or replication task that is on hold counts as failed: it cannot run until somebody puts right what is wrong. A scrub has only `truenas_task_enabled`, since how it went is the pool's scan.

### What is always there and what comes and goes

These are written at 0 when there is nothing to count, so a graph shows none and an alert has something to watch before the first failure: `truenas_alerts` for every level, `truenas_jobs` for both states, `truenas_apps` and `truenas_vms` for every state, `truenas_shares` for every protocol and state, the four `truenas_tasks` counts for every kind, `truenas_system_state` for every state, and `truenas_pool_scan_running` for both kinds of scan.

These are on the page only while there is something to say: `truenas_alerts_by_class`, `truenas_jobs_running`, the readings of each app, virtual machine, task, dataset and disk, a scan's readings on a pool that has been scanned, a task's last run once it has run, a quota where one is set, a temperature where a disk gives one.

A state or level a newer TrueNAS adds is written under its own name rather than left out.

The page holds nothing but these: none of the Go runtime's or the process's own readings.

### Labels of your own

`TRUENAS_METRICS_LABELS` puts labels on every reading, for a store that tells one source from another by them:

```env
TRUENAS_METRICS_LABELS=hostname=nas,site=home
```

```text
truenas_pool_healthy{hostname="nas",pool="tank",site="home"} 1
```

A label named as one a reading already has (`pool`, `disk`, `state` and so on) stops the program as it starts, saying which.

`job` and `instance` can be set this way too, but whatever scrapes the page sets its own and renames the page's to `exported_job` and `exported_instance`. To keep the page's, tell the scraper to: `honor_labels: true` in Prometheus, vmagent and Alloy's `prometheus.scrape`.

### When the NAS stops answering

A scrape is answered from memory and never waits on the NAS. When a round of questions fails, `truenas_up` goes to 0 and each reading stays as it last was for three of its intervals, then is left off the page rather than shown as current. One question failing does not cost the others their answers.

A method the NAS does not have, as an older or newer TrueNAS may not, is logged once and its readings are left off.

## Installation

```bash
docker pull ghcr.io/katbyte/truenas-metrics:latest
```

or

```bash
go install github.com/katbyte/truenas-metrics@latest
```

or

```bash
brew install katbyte/tap/truenas-metrics
```

## Configuration

All options can be passed as command-line flags, environment variables, or via a configuration file.

| Variable | Flag | Description |
|---|---|---|
| `TRUENAS_METRICS_SERVER` | `--server` | the NAS's name or address, e.g. `nas.example.net` or `https://192.168.1.5`; it is only ever reached over TLS |
| `TRUENAS_METRICS_USERNAME` | `--username` | the user the API key was made for |
| `TRUENAS_METRICS_API_KEY` | `--api-key` | the API key |
| `TRUENAS_METRICS_CERTIFICATE_FILE` | `--certificate-file` | the NAS's own certificate as a PEM file: the setting for a NAS showing the certificate it came with |
| `TRUENAS_METRICS_CA_FILE` | `--ca-file` | a PEM file of certificates to trust as well as the system's, for a NAS given a certificate with its own name on it |
| `TRUENAS_METRICS_SKIP_CERTIFICATE_CHECK` | `--skip-certificate-check` | take whatever certificate the NAS shows |
| `TRUENAS_METRICS_SEND_KEY_UNCHECKED` | `--send-key-unchecked` | with the one above, send the key to a TrueNAS before version 26 all the same |
| `TRUENAS_METRICS_LISTEN` | `--listen` | the address the page is served on, default `:9089` |
| `TRUENAS_METRICS_LABELS` | `--labels` | labels to put on every reading, written `name=value,name=value` |
| `TRUENAS_METRICS_FAST_INTERVAL` | `--fast-interval` | how often pools and their scans, alerts, services, jobs, apps and the NAS's own state are read, default `30s` |
| `TRUENAS_METRICS_SLOW_INTERVAL` | `--slow-interval` | how often disks, datasets, tasks, shares, certificates and updates are read, default `5m` |
| `TRUENAS_METRICS_DATASET_DEPTH` | `--dataset-depth` | how far below its pool a dataset may be and still be named, default `1` |
| `TRUENAS_METRICS_LOG` | | log level (`WARN` default): `INFO` adds what is being reported on and when the NAS comes back, `DEBUG` what the connection is doing |
| `TRUENAS_METRICS_CONFIG` | `--config` | read settings from this file and no other, in place of `./.truenas-metrics` or `~/.truenas-metrics` |

### The user and the key

Make the program a user of its own on the NAS with the read-only administrator role, and make that user an API key. Every question it asks is one that role may ask, and it can change nothing.

The key is never logged and never on the page. `truenas_api_key_expiry_timestamp_seconds` says when a key that expires will stop working, in time to make another.

### The NAS's certificate

A TrueNAS revokes a key that is sent to it without TLS, so the program refuses an address that starts `http://` or `ws://` before sending anything.

A NAS as it comes shows a certificate it made itself, made out to `localhost`, which is not the name anything reaches it by. So the program is told which certificate is the NAS's, one of three ways:

- **`--certificate-file`** is the NAS's own certificate, and the program speaks only to a NAS that holds its key, whatever name and dates are on it. This is the one for a NAS as it comes. Fetch the certificate once, from a machine you trust to be talking to the NAS:

  ```bash
  openssl s_client -connect nas.example.net:443 </dev/null 2>/dev/null | openssl x509 > nas.pem
  ```

  A certificate renewed with the same key goes on working. One made with a new key is refused, with a line in the log saying so, until the file is replaced. Since the dates on it are not checked, `truenas_certificate_expiry_timestamp_seconds` is what warns of one running out.
- **`--ca-file`** is for a NAS that has been given a certificate with its own name on it: the certificate, or the authority that signed it, is trusted beside the system's own, and the name is checked as usual.
- **`--skip-certificate-check`** takes whatever certificate is shown. The connection is still encrypted, but nothing says it is the NAS at the other end. A TrueNAS from version 26 on is logged into without the key ever being sent, so this is enough for one. A TrueNAS before that takes a key only as it is, and is not sent it: the program fails to log in and says why, unless `--send-key-unchecked` is given too, which hands the key to whatever answers at the address. The program says so in its log each time it starts.

Set one of the three and no more. With none, the NAS's certificate has to be one the system already trusts.

### Configuration File

You can place a `.truenas-metrics` file in your home directory `~/.truenas-metrics` or in your current directory `./.truenas-metrics`. Keys match the long flag names using the `env` format, with an underscore between words:

```env
SERVER=nas.example.net
USERNAME=metrics
API_KEY=3-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
CERTIFICATE_FILE=/etc/truenas-metrics/nas.pem
```

A flag or an environment variable still wins over the file. `--config` (or `TRUENAS_METRICS_CONFIG`) names the file to read, and it is then the only one read; a file that is named and is not there stops the program from starting.

## Usage

```bash
truenas-metrics print   # ask once and print the page: a check that the address, the user and the key work
truenas-metrics serve   # serve the page at /metrics, with a health probe at /healthz
```

### Docker

[`docker-compose.yml`](docker-compose.yml) runs the released image, with the key in a `.env` beside it ([`.env.example`](.env.example)):

```bash
cp .env.example .env   # and put the key in it
openssl s_client -connect nas.example.net:443 </dev/null 2>/dev/null | openssl x509 > nas.pem
docker compose up -d
curl -s http://127.0.0.1:9089/metrics
```

The example publishes the port on `127.0.0.1` only, since the page needs no password and whatever scrapes it usually runs on the same host.

### Scraping it

Grafana Alloy:

```alloy
prometheus.scrape "truenas" {
  targets         = [{"__address__" = "127.0.0.1:9089"}]
  job_name        = "truenas"
  scrape_interval = "30s"
  forward_to      = [prometheus.remote_write.default.receiver]
}
```

Prometheus, vmagent or VictoriaMetrics' own scraper:

```yaml
scrape_configs:
  - job_name: truenas
    scrape_interval: 30s
    static_configs:
      - targets: ['127.0.0.1:9089']
```

Scraping more often than the NAS is asked gains nothing: the page only changes when a round of questions comes back.

A few things to ask of it:

```promql
truenas_up == 0                                                    # the NAS is not answering
truenas_pool_healthy == 0                                          # a pool is not healthy
truenas_pool_device_status{status!="ONLINE"}                       # a disk in a pool is not online
truenas_pool_device_read_errors + truenas_pool_device_write_errors + truenas_pool_device_checksum_errors > 0   # errors against a disk
time() - truenas_pool_scan_end_timestamp_seconds{function="SCRUB"} > 40 * 86400   # no scrub has finished in 40 days
truenas_pool_allocated_bytes / truenas_pool_size_bytes > 0.8       # a pool is over 80% full
truenas_alerts{level=~"CRITICAL|ALERT|EMERGENCY"} > 0              # the NAS has a serious alert
truenas_tasks_failed > 0                                           # a scheduled task failed its last run
truenas_task_last_run_failed == 1                                  # which one
time() - truenas_task_last_run_timestamp_seconds{kind="replication"} > 2 * 86400   # a replication has not run in two days
truenas_service_enabled == 1 and truenas_service_running == 0      # a service that should be running is not
truenas_certificate_expiry_timestamp_seconds - time() < 14 * 86400 # a certificate runs out within two weeks
truenas_api_key_expiry_timestamp_seconds - time() < 14 * 86400     # so does the key this uses
truenas_disk_temperature_celsius > 50                              # a disk is hot
```

To put a disk's serial number beside a reading from somewhere else that has only its name, or its slot beside one of these, join on what both have:

```promql
truenas_disk_temperature_celsius * on (serial) group_left (model, pool) truenas_disk_info
```

## Development

```bash
make            # fmt + build
make test       # the tests, under the race detector
make test-live  # the live tests, against a real TrueNAS
make testacc    # the same against a throwaway TrueNAS in a virtual machine, on Linux with KVM
make check-all  # build + tests + every linter + depscheck
```

The tests need no TrueNAS. The page's are run against a NAS the test makes up, and the whole page is kept in [`lib/readings/testdata/page.txt`](lib/readings/testdata/page.txt), so a reading that is renamed shows up as a difference there; `go test ./lib/readings -update` rewrites it once the change is meant. What a real NAS answers is kept in [`lib/nas/testdata`](lib/nas/testdata), one file to each method the program calls, with every name and serial number in it made up; one test runs the whole program against a stand-in that speaks the API's protocol and answers with those.

`make test-live` runs the program against a real TrueNAS and holds the page to what that NAS says when asked directly: every pool and its status, every disk by serial number, every dataset and service, and as many alerts, tasks and shares as it lists. It only reads and knows nothing of the NAS beforehand, so any TrueNAS will do; a throwaway one in a virtual machine is the place to break a pool on purpose and see it reported. Name the NAS in the environment, logging in as the read-only user the program is meant to run as:

```bash
export TRUENAS_METRICS_TEST_SERVER=192.168.1.50
export TRUENAS_METRICS_TEST_USERNAME=metrics
export TRUENAS_METRICS_TEST_API_KEY=3-xxxxxxxx
export TRUENAS_METRICS_TEST_CERTIFICATE_FILE=./nas.pem
make test-live
```

A `.testenv.sh` holding those four lines is ignored by git, as is any `.pem`.

`make testacc` needs no NAS of yours: it fetches [truenas-mcp](https://github.com/katbyte/truenas-mcp)'s test environment, has it install TrueNAS in a virtual machine, runs the same suite against it as a read-only user, and removes it. That takes qemu and an x86-64 Linux machine with `/dev/kvm`, which is what the acceptance workflow runs on; TrueNAS has no build for a Mac's processor. The first run installs TrueNAS, which takes a quarter of an hour; later runs start from the installed disk.

How it is put together:

- [`lib/poll`](lib/poll) asks the questions on two schedules and keeps the last good answer to each, in the program's own words for what a NAS says.
- [`lib/nas`](lib/nas) is the only part that knows the API: which methods answer each question and how to read what they say. The list of methods it may call is in its tests, and a method not on the list fails them.
- [`lib/readings`](lib/readings) turns what the poller knows into the page.
- The connection to the NAS is [truenas-mcp](https://github.com/katbyte/truenas-mcp)'s.

Dependencies are vendored, and the dev tools are pinned in `.tools/go.mod` and the makefile; `make tools` installs them into `.tools/bin`.
