# Queries

Alerts and queries to start from. Every reading is in [readings.md](readings.md).

```promql
truenas_up == 0                                                    # the NAS is not answering
time() - truenas_poll_last_success_timestamp_seconds > 3 * truenas_poll_interval_seconds   # a part of the NAS has gone unanswered three polls running
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
abs(truenas_system_clock_offset_seconds) > 2                       # the NAS's clock has wandered
```

## Joining on a disk's serial number

Every disk reading carries the serial. Use it to put a disk's model or pool beside another reading, from this exporter or any other:

```promql
truenas_disk_temperature_celsius * on (serial) group_left (model, pool) truenas_disk_info
```

## How often to scrape

Scraping more often than the NAS is asked gains nothing. The page only changes when a poll comes back: every 30 seconds for what changes quickly, every 5 or 10 minutes for most of the rest, or as `--intervals` says.
