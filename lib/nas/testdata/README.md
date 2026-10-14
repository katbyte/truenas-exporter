# testdata

One file per TrueNAS API method the exporter calls, holding what a NAS answers.

Most came from a real TrueNAS 25.10.7 and were then scrubbed:

- Shapes, states, sizes and hardware models are as the NAS gave them.
- Every name, serial number, path and piece of text is made up.
- Lists were cut to a few rows.
- Other people's words were left out: an alert's text, a task's log, a share's settings.

These seven were written from the API's own description, because that NAS had none of the thing: `app.query`, `vm.query`, `replication.query`, `cloud_backup.query`, `sharing.nfs.query`, `iscsi.target.query` and `api_key.my_keys`. They are the least certain.

Never put a real answer here as it came. It names disks by serial number, and datasets and shares by what their owner called them.
