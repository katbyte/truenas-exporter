# Changelog

## 0.3.2 (2026-10-18)

- **The log says which version is running** as `print` and `serve` start, at `INFO`: `starting truenas-exporter v0.3.2`, then the commit it was built from in brackets. `truenas-exporter version` now names the commit too.

## 0.3.1 (2026-10-18)

- **The readings of how the asking goes can be left off**: `--skip poll` drops the four `truenas_poll_*` readings, and `--metrics` keeps them only when it names `poll`. `truenas_up` stays either way.

## 0.3.0 (2026-10-18)

Built on go-kt's shared exporter pieces, which the other exporters use too.

- **The build reading takes the program's new name**: `truenas_exporter_build_info`, in place of `truenas_metrics_build_info`.
- **The four `truenas_poll_*` readings say what they are in new words**, and a part has none until it has been asked once. Their names and labels are unchanged.
- **`print` names the parts of the NAS that did not answer** when it fails, and the log says "the NAS: the pools round failed".
- **`--intervals` refuses a part named twice.**

## 0.2.0 (2026-10-15)

Renamed to truenas-exporter, as nearly every Prometheus exporter is named.

- **The program, its Docker image and its Homebrew formula are now `truenas-exporter`**: `ghcr.io/katbyte/truenas-exporter` and `brew install katbyte/tap/truenas-exporter`.
- **Every setting is renamed, and the old names no longer work**: `TRUENAS_METRICS_*` is now `TRUENAS_EXPORTER_*`, and the settings file is `.truenas-exporter` in place of `.truenas-metrics`.
- **The readings keep their names.** That includes `truenas_metrics_build_info` for now.

## 0.1.1 (2026-10-14)

- **The NAS's clock reading is centred.** TrueNAS gives its time to the whole second, so `truenas_system_clock_offset_seconds` read 0 to 1 second low. Half a second is now added, which `--clock-correction` changes.
- **The certificate file is optional in the Docker example.** A NAS reached by a name whose certificate is already trusted, through a proxy say, needs none.

## 0.1.0 (2026-10-14)

The first release: a metrics page for a TrueNAS.

- **Whether the storage is healthy, as a page Prometheus can read**: each pool's health, how full it is, and the errors against each vdev and each disk in it, the boot pool included.
- **Scrubs and resilvers**: whether one is running, how far it has got, when the last one finished and what it found.
- **Space by dataset**, with quotas and what snapshots hold on to, and how many datasets and snapshots each pool has.
- **Alerts** by how serious they are and of what kind.
- **Scheduled tasks**: snapshots, replication, cloud sync, cloud backup and rsync, with which are running, which failed last time and when each last ran.
- **What is running**: services, jobs, apps and virtual machines.
- **Disks** by serial number, with their temperature and which pool they are in, so readings of the same disk from elsewhere can be joined on.
- **The small things that bite**: certificates and the API key running out, an update waiting, a network link down.
- **Nobody's name is on it**: no share, path, user or client, and datasets are named only as deep as you say.
- **It only reads**, over the NAS's API and from another machine. Nothing is installed on the NAS, and a NAS that is slow or down never holds up a scrape.
- **`serve` keeps the page up** for a scraper to read, and `print` shows it once as a check that the NAS can be reached.
- **Ask about only what you want, as often as you want**: `--metrics` and `--skip` choose which parts of the NAS are asked about, and `--intervals` how often each is. By default the quick-changing parts are asked every 30 seconds, most of the rest every 5 minutes, snapshot counts every 10 and the update check every 4 hours.
- **Labels of your own on every reading**, to tell one NAS from another.
- Ships as a Docker image, a Homebrew formula and ready-built programs.
