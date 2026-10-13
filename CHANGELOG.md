# Changelog

## 0.1.0 (unreleased)

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
- **Labels of your own on every reading**, to tell one NAS from another.
- Ships as a Docker image, a Homebrew formula and ready-built programs.
