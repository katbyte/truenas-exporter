# Settings

The [README](../README.md#settings) lists every setting. This page has the detail.

## The user and the key

Make the program a user of its own on the NAS, with the read-only administrator role. Make that user an API key.

Every question the program asks is one that role may ask. It can change nothing.

The key is never logged and never on the page. `truenas_api_key_expiry_timestamp_seconds` says when a key that expires will stop working, in time to make another.

## The NAS's certificate

TrueNAS revokes a key that is sent to it without encryption. So the program refuses an address that starts `http://` or `ws://` before sending anything.

A new TrueNAS shows a certificate it made itself, made out to `localhost`. That is not the name anything reaches it by. So the program is told which certificate is the NAS's, in one of three ways. Set one and no more.

### `--certificate-file`

The NAS's own certificate. The program speaks only to a NAS that holds that certificate's key, whatever name and dates are on it. This is the one for a NAS as it comes.

Fetch the certificate once, from a machine you trust to be talking to the NAS:

```bash
openssl s_client -connect nas.example.net:443 </dev/null 2>/dev/null | openssl x509 > nas.pem
```

- A certificate renewed with the same key goes on working.
- One made with a new key is refused, with a line in the log saying so, until the file is replaced.
- The dates on it are not checked. `truenas_certificate_expiry_timestamp_seconds` is what warns of one running out.

### `--ca-file`

For a NAS that has been given a certificate with its own name on it. The certificate, or the authority that signed it, is trusted beside the system's own. The name is checked as usual.

### `--skip-certificate-check`

Takes whatever certificate is shown. The connection is still encrypted, but nothing says it is the NAS at the other end.

- TrueNAS 26 and later log in without the key ever being sent, so this setting is enough.
- Before TrueNAS 26 the key itself has to be sent, and the program will not send it unchecked. It fails to log in and says why.
- `--send-key-unchecked` sends it anyway, to whatever answers at the address. The program says so in its log each time it starts.

### None of them

The NAS's certificate has to be one the system already trusts.

## The settings file

The program reads `~/.truenas-metrics` or `./.truenas-metrics`. Keys are the long flag names in capitals, with an underscore between words:

```env
SERVER=nas.example.net
USERNAME=metrics
API_KEY=3-xxxxxxxx
CERTIFICATE_FILE=/etc/truenas-metrics/nas.pem
```

A flag or an environment variable wins over the file.

`--config` (or `TRUENAS_METRICS_CONFIG`) names the file to read. It is then the only one read. A named file that is not there stops the program from starting.

## What the NAS is asked about, and how often

Each part of the NAS below is asked about on its own schedule. Three settings change that, each a list with commas between:

- **`--metrics`** names the parts to ask about, like `pools,disks`. Left unset, it is all of them.
- **`--skip`** leaves parts out of those, like `apps,vms`.
- **`--intervals`** says how often to ask about a part, like `snapshots=30m,apps=1m`. The rest keep the times below.

A part left out is never asked about and has nothing on the page.

| Part | What it covers | Every |
|---|---|---|
| `system` | the NAS's own state: version, uptime, load | 30s |
| `pools` | pools and the boot pool, with their scans, vdevs and the disks in them | 30s |
| `disks` | every disk, its size and temperature | 5m |
| `datasets` | datasets and their space | 5m |
| `snapshots` | how many snapshots each dataset has | 10m |
| `alerts` | alerts | 30s |
| `services` | services | 30s |
| `jobs` | jobs running or waiting | 30s |
| `apps` | apps and the apps service | 30s |
| `vms` | virtual machines | 30s |
| `shares` | shares | 5m |
| `clients` | clients connected to the shares | 30s |
| `tasks` | scheduled tasks | 5m |
| `updates` | whether a newer TrueNAS is out | 5m |
| `interfaces` | network interfaces | 5m |
| `certificates` | when certificates expire | 5m |
| `directory` | the directory service | 5m |
| `key-expiry` | when the program's API key expires | 5m |

- `system` is always asked about, since it is what says the NAS is answering. It can be given its own time, but not left out.
- Counting snapshots has the NAS go through every one, so it is asked least often.
- A word that names no part, or a time that cannot be read, stops the program at start.
- Pools give each disk's serial number only while `disks` is asked, and disks give their pool only while `pools` is.

## Logging

`TRUENAS_METRICS_LOG` sets the log level. `WARN` is the default. `INFO` adds what is being reported on and when the NAS comes back. `DEBUG` adds what the connection is doing.

## Docker

[`docker-compose.yml`](../docker-compose.yml) runs the released image, with the key in a `.env` beside it ([`.env.example`](../.env.example)):

```bash
cp .env.example .env   # put the key in it
openssl s_client -connect nas.example.net:443 </dev/null 2>/dev/null | openssl x509 > nas.pem
docker compose up -d
curl -s http://127.0.0.1:9089/metrics
```

The page has no password, so the example serves it on `127.0.0.1` only. Whatever scrapes it usually runs on the same host.

## Labels of your own

See [Labels of your own](readings.md#labels-of-your-own).
