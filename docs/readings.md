# Readings

Every reading starts `truenas_`. Statuses, states and levels are spelled as TrueNAS spells them: `ONLINE`, `RUNNING`, `CRITICAL`.

## Polling

| Reading | Labels | What it says |
|---|---|---|
| `truenas_up` | | 1 when the NAS answered the last time it was asked about its own state |
| `truenas_poll_interval_seconds` | `poll` | how often each part is asked about |
| `truenas_poll_duration_seconds` | `poll` | how long the last poll of each part took |
| `truenas_poll_last_success_timestamp_seconds` | `poll` | when each part was last answered |
| `truenas_poll_failures_total` | `poll` | failed polls of each part since start |
| `truenas_metrics_build_info` | `version`, `commit`, `goversion` | always 1; the version running |

`poll` is the part of the NAS, named as [the settings](settings.md#what-the-nas-is-asked-about-and-how-often) name it: `system`, `pools`, `snapshots` and the rest.

## The NAS

| Reading | Labels | What it says |
|---|---|---|
| `truenas_system_info` | `version`, `product`, `manufacturer`, `cpu` | always 1; TrueNAS version and hardware |
| `truenas_system_state` | `state` | 1 for the current state: `BOOTING`, `READY`, `SHUTTING_DOWN` |
| `truenas_system_boot_timestamp_seconds` | | when it last started |
| `truenas_system_uptime_seconds` | | how long it has been up |
| `truenas_system_memory_bytes` | | memory installed |
| `truenas_system_cpu_cores`, `truenas_system_cpu_physical_cores` | | processor cores, with and without threads |
| `truenas_system_load_average` | `window` | load over `1m`, `5m` and `15m` |
| `truenas_system_ecc_memory` | | 1 when memory is error-correcting |
| `truenas_system_clock_offset_seconds` | | the NAS's clock minus this machine's |
| `truenas_update_available` | | 1 when a newer TrueNAS is out |
| `truenas_update_check_failed` | | 1 when the update check failed |
| `truenas_interface_link_up` | `interface`, `type` | 1 when the interface has a link |
| `truenas_interface_mtu_bytes` | `interface` | the interface's largest packet size |
| `truenas_certificate_expiry_timestamp_seconds` | `certificate` | when each certificate expires |
| `truenas_directory_service_healthy` | `type`, `status` | 1 when the joined directory is healthy |
| `truenas_api_key_expiry_timestamp_seconds` | | when this program's key expires, if it does |

- The NAS gives its time to the whole second, so half a second is added to centre the clock offset, which is then good to within half a second. `--clock-correction` changes what is added.
- The directory reading is only there on a NAS joined to a directory.

## Pools

The boot pool is listed as `boot-pool`.

| Reading | Labels | What it says |
|---|---|---|
| `truenas_pool_healthy` | `pool` | 1 when the pool is healthy |
| `truenas_pool_warning` | `pool` | 1 when the pool has a warning |
| `truenas_pool_status` | `pool`, `status` | always 1; `ONLINE`, `DEGRADED`, `FAULTED`, `OFFLINE`, `UNAVAIL` or `REMOVED` |
| `truenas_pool_size_bytes`, `truenas_pool_allocated_bytes`, `truenas_pool_free_bytes` | `pool` | size, space written and space free, parity included |
| `truenas_pool_freeing_bytes` | `pool` | space still being given back after a delete |
| `truenas_pool_fragmentation_ratio` | `pool` | how broken up free space is, 0 to 1 |
| `truenas_pool_scan_running` | `pool`, `function` | 1 while a `SCRUB` or `RESILVER` runs |
| `truenas_pool_scan_progress_ratio` | `pool`, `function` | how far the last scan got, 0 to 1 |
| `truenas_pool_scan_errors` | `pool`, `function` | errors the last scan found |
| `truenas_pool_scan_start_timestamp_seconds`, `truenas_pool_scan_end_timestamp_seconds` | `pool`, `function` | when the last scan started and ended |
| `truenas_pool_scan_to_examine_bytes`, `truenas_pool_scan_examined_bytes`, `truenas_pool_scan_issued_bytes` | `pool`, `function` | bytes to check, looked at, and checked |
| `truenas_pool_scan_remaining_seconds` | `pool`, `function` | time a running scan has left |
| `truenas_pool_scan_paused` | `pool`, `function` | 1 when the scan is paused |
| `truenas_pool_vdev_info` | `pool`, `class`, `vdev`, `type` | always 1; the vdev's kind: `RAIDZ2`, `MIRROR`, `DISK` |
| `truenas_pool_vdev_status` | `pool`, `class`, `vdev`, `status` | always 1; the vdev's status |
| `truenas_pool_vdev_size_bytes`, `truenas_pool_vdev_allocated_bytes`, `truenas_pool_vdev_fragmentation_ratio` | `pool`, `class`, `vdev` | size, use and fragmentation of each data vdev |
| `truenas_pool_vdev_read_errors`, `truenas_pool_vdev_write_errors`, `truenas_pool_vdev_checksum_errors` | `pool`, `class`, `vdev` | errors on the vdev since last cleared |
| `truenas_pool_device_status` | `pool`, `class`, `vdev`, `disk`, `serial`, `status` | always 1; the disk's status in its pool |
| `truenas_pool_device_read_errors`, `truenas_pool_device_write_errors`, `truenas_pool_device_checksum_errors` | `pool`, `class`, `vdev`, `disk`, `serial` | errors on the disk since last cleared |

- `class` is `data`, `log`, `cache`, `spare`, `special` or `dedup`.
- A vdev of one disk takes the disk's name.
- A pool remembers only its last scan, scrub or resilver.
- A scan's end time is left off while it runs.

## Disks

| Reading | Labels | What it says |
|---|---|---|
| `truenas_disk_info` | `disk`, `serial`, `wwn`, `model`, `type`, `bus`, `pool`, `enclosure`, `slot` | always 1; what is known of each disk |
| `truenas_disk_size_bytes` | `disk`, `serial` | the disk's size |
| `truenas_disk_temperature_celsius` | `disk`, `serial` | the disk's temperature, where it gives one |

`disk` is the NAS's name for it, like `sda`, and can change on restart. `serial` does not. `enclosure` and `slot` are empty when the NAS does not know them.

To put a disk's model or pool beside another reading, join on the serial:

```promql
truenas_disk_temperature_celsius * on (serial) group_left (model, pool) truenas_disk_info
```

## Datasets and snapshots

| Reading | Labels | What it says |
|---|---|---|
| `truenas_datasets` | `pool` | datasets in the pool, at any depth |
| `truenas_snapshots` | `pool` | snapshots in the pool, at any depth |
| `truenas_dataset_used_bytes` | `dataset`, `pool` | space used, children and snapshots included |
| `truenas_dataset_available_bytes` | `dataset`, `pool` | space it can still grow by |
| `truenas_dataset_used_by_dataset_bytes` | `dataset`, `pool` | space its own data uses |
| `truenas_dataset_used_by_snapshots_bytes` | `dataset`, `pool` | space its snapshots hold |
| `truenas_dataset_quota_bytes`, `truenas_dataset_refquota_bytes` | `dataset`, `pool` | its quotas, where set |
| `truenas_dataset_compression_ratio` | `dataset`, `pool` | how many times compression shrinks it |
| `truenas_dataset_snapshots` | `dataset`, `pool` | snapshots of this dataset itself |

Datasets are named one level below the pool, since deeper names are often people's. `TRUENAS_EXPORTER_DATASET_DEPTH` changes that; `0` names only the pool's own. The two counts always include every dataset.

Snapshot counts are asked for apart from the rest, every 10 minutes unless `--intervals` says otherwise: counting them has the NAS go through every snapshot.

## Alerts

| Reading | Labels | What it says |
|---|---|---|
| `truenas_alerts` | `level` | undismissed alerts by level: `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`, `ALERT`, `EMERGENCY` |
| `truenas_alerts_by_class` | `level`, `class` | the same by kind, like `RsyncFailed` |
| `truenas_alerts_dismissed` | | dismissed alerts the NAS still holds |

Alert text is not on the page: it names paths and hosts.

## Services, jobs, apps and virtual machines

| Reading | Labels | What it says |
|---|---|---|
| `truenas_service_running` | `service` | 1 when the service is running |
| `truenas_service_enabled` | `service` | 1 when it starts with the NAS |
| `truenas_jobs` | `state` | unfinished jobs: `RUNNING`, `WAITING` |
| `truenas_jobs_running` | `method` | running jobs by what they do, like `cloudsync.sync` |
| `truenas_apps_service_status` | `status` | always 1; the state of the apps service |
| `truenas_apps` | `state` | apps by state: `RUNNING`, `DEPLOYING`, `STOPPING`, `STOPPED`, `CRASHED` |
| `truenas_app_state` | `app`, `state` | always 1; each app's state |
| `truenas_app_update_available` | `app` | 1 when the app has an update |
| `truenas_vms` | `state` | virtual machines by state: `RUNNING`, `STOPPED`, `SUSPENDED` |
| `truenas_vm_state` | `vm`, `state` | always 1; each virtual machine's state |

- `service` is TrueNAS's name for it: `cifs`, `nfs`, `ssh` and the rest.
- The apps service is `UNCONFIGURED` where apps were never set up.

## Shares

| Reading | Labels | What it says |
|---|---|---|
| `truenas_shares` | `protocol`, `state` | shares over `smb`, `nfs` and `iscsi`: `enabled`, `disabled` or `locked` |
| `truenas_share_clients` | `protocol` | clients connected over `nfs` and `iscsi` |

A share is `locked` when its dataset is. TrueNAS gives no client count for SMB. Share names, paths and who is connected are never on the page.

## Scheduled tasks

`kind` is `snapshot`, `replication`, `cloud_sync`, `cloud_backup`, `rsync` or `scrub`.

| Reading | Labels | What it says |
|---|---|---|
| `truenas_tasks` | `kind` | how many tasks there are |
| `truenas_tasks_enabled` | `kind` | how many are switched on |
| `truenas_tasks_running` | `kind` | how many are running |
| `truenas_tasks_failed` | `kind` | how many failed their last run |
| `truenas_task_enabled` | `kind`, `id`, `name` | 1 when the task is switched on |
| `truenas_task_running` | `kind`, `id`, `name` | 1 while it runs |
| `truenas_task_progress_ratio` | `kind`, `id`, `name` | how far a running task has got, 0 to 1 |
| `truenas_task_last_run_failed` | `kind`, `id`, `name` | 1 when its last run failed |
| `truenas_task_last_run_timestamp_seconds` | `kind`, `id`, `name` | when its last run ended |
| `truenas_task_last_run_duration_seconds` | `kind`, `id`, `name` | how long its last run took |

- `id` is the task's number in TrueNAS. `name` is what its maker called it.
- A snapshot task is named by its dataset, a scrub by its pool.
- A run stopped by hand did not fail.
- A snapshot or replication task on hold counts as failed.
- Duration is given for cloud sync, cloud backup and rsync only.
- A scrub has only `truenas_task_enabled`. How it went is the pool's scan.

## Always there, or only sometimes

These counts are written as 0 when there is nothing to count, so an alert has something to watch: `truenas_alerts`, `truenas_jobs`, `truenas_apps`, `truenas_vms`, `truenas_shares`, the four `truenas_tasks` counts, `truenas_system_state` and `truenas_pool_scan_running`.

The rest appear only when there is something to say: `truenas_alerts_by_class`, `truenas_jobs_running`, readings of each app, virtual machine, task, dataset and disk, a scan's details, a quota, a temperature.

A part of the NAS left out with `--metrics` or `--skip` has nothing on the page, its counts included. See [settings](settings.md#what-the-nas-is-asked-about-and-how-often).

A state a newer TrueNAS adds is written under its own name. The page has no Go runtime or process readings.

## Labels of your own

`TRUENAS_EXPORTER_LABELS` adds labels to every reading:

```env
TRUENAS_EXPORTER_LABELS=hostname=nas,site=home
```

```text
truenas_pool_healthy{hostname="nas",pool="tank",site="home"} 1
```

A label a reading already uses, like `pool`, stops the program at start.

A scraper replaces `job` and `instance` with its own unless told `honor_labels: true`.

## When the NAS stops answering

A scrape is answered from memory and never waits on the NAS. If the NAS stops answering, `truenas_up` goes to 0. One part failing leaves `truenas_up` at 1 and shows in that part's `truenas_poll_failures_total`. Each part's readings stay for three of its intervals, then drop off the page. One question failing does not cost the others their answers.

A method this TrueNAS does not have is logged once, and its readings are left off.
