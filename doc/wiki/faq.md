# Frequently Asked Questions

Common questions, problems, and fixes for VoiceWatch. For step-by-step guides see the [Installation Guide](installation.md), [Recommended Hardware](hardware.md), [RTSP Troubleshooting](rtsp-troubleshooting.md), [Detection Pipeline](detection-pipeline.md), and [Remote Access / Cloudflare Tunnel](cloudflare_tunnel_guide.md).

If your problem isn't covered here, generate a support dump from **Settings > Support** (it bundles your config and logs) and open a [discussion](https://github.com/tphakala/voicewatch/discussions) or [issue](https://github.com/tphakala/voicewatch/issues).

## Read this first: a few things that explain most problems

These come up again and again, so they are worth knowing before anything else:

- **Run `install.sh` as your normal user, never with `sudo`.** Running the installer as root creates a second, separate instance under `/root/birdnet-go-app` while your real data stays under `~/birdnet-go-app`. This is the single most common cause of "an update deleted my settings/history". Nothing was deleted; you were just looking at a fresh root install. Re-run the installer as your normal user and your data is there.
- **Use a single mono microphone (or a mono stream).** Stereo audio does not improve detection and can hurt it. A mono mic is recommended for both local capture and RTSP.
- **Settings hot-reload.** Almost all settings now take effect immediately without a restart. If a setting change does not seem to apply, you are probably on an older build; update to the latest nightly.

## Installation and updates

### How do I update VoiceWatch? Will I lose my data?

Re-run `install.sh` as your normal user and choose **"Check for updates"** (option 1). It pulls the latest image and restarts; it does not touch your database or config. The only destructive option is "Fresh installation", which warns you and requires typing `yes`. Again: never run the installer with `sudo`.

### My detection history is empty after an update

The app opened a fresh, empty database instead of your real one. Your old detections are still in `birdnet.db`, do not delete it. This is usually caused by the data directory not being where the app expected (often the sudo-vs-normal-user issue above). Run the [Database Doctor](https://github.com/tphakala/voicewatch/wiki/Database-Doctor): diagnose, then `--fix` (it backs up the DB first).

### Does install.sh work on macOS?

No. `install.sh` supports Debian, Ubuntu, and Raspberry Pi OS only. Use a manual binary install on macOS (see the [Installation Guide](installation.md)).

### I installed as root. How do I move to a normal user without losing data?

Run `install.sh` as your normal user; it detects the root install and offers to migrate it automatically. After migrating, the old `/root/birdnet-go-app` is left in place so you can verify, which makes the installer keep re-offering migration. Once you have confirmed everything works, remove it:

```bash
sudo rm -rf /root/birdnet-go-app
```

### Are the DietPi packages official?

No. DietPi packaging is maintained by the DietPi project, not here.

### Why are my times shown in UTC (or the wrong timezone)?

VoiceWatch displays times in the timezone of the server/container it runs in, not your browser's. The `install.sh` setup sets this for you; for manual Docker or Docker Compose deployments you need to set `TZ` yourself (e.g. `TZ=America/New_York` in the container environment). On a native/systemd install, set the host timezone with `sudo timedatectl set-timezone Area/City`. There is no per-viewer browser conversion, so a UTC container shows UTC to everyone.

## Microphones and sound cards

### My USB sound card stops working after a reboot (I have to reselect it every time)

This is a known limitation. Linux assigns ALSA card numbers in USB enumeration order at boot, so your USB mic can come up as a different card number on different boots. VoiceWatch currently saves the device by that card number, so after a renumber the saved selection no longer matches and capture fails. A startup delay does **not** help, because the device is present, just under a different number.

Workaround: pin the card to a fixed ALSA index on the host. Find the USB IDs with `lsusb`, then create `/etc/modprobe.d/alsa-birdnet.conf`:

```
options snd-usb-audio index=0 vid=0x16d0 pid=0x06f3
```

(Replace the IDs with yours; drop `vid`/`pid` if it is your only USB sound card.) Then `sudo update-initramfs -u`, reboot, and reselect the card once. Host-level pinning carries into the Docker container through `--device /dev/snd`. A proper fix (matching the card on a stable USB hardware id) is planned.

### I can't add an audio device, the Save button stays greyed out

Fill in the **Name** field for the source. Save stays disabled until the source has a name.

### Which microphone should I use?

See [Recommended Hardware](hardware.md). Popular community choices include a lavalier mic (e.g. Boya USB BY-LM40) or a DIY capsule (PUI Audio AOM-5024L-HD-R) on a cheap USB sound card. Keep the mic out of direct rain.

### Can I save recordings to an external USB drive?

Yes. Set an absolute path in **Settings > Audio > Export > Clips Folder** (e.g. `/mnt/usb/voicewatch/`). Because install.sh runs VoiceWatch in a container, you must also mount the drive on the host and bind-mount it into the container's systemd service (`sudo systemctl edit birdnet-go.service --full`, add `-v /mnt/usb:/mnt/usb`), then make sure the mount is writable by UID 1000.

## RTSP cameras and streams

See [RTSP Troubleshooting](rtsp-troubleshooting.md) for the full guide.

### My stream works in VLC but VoiceWatch shows it as "Unknown" or "Unhealthy"

Most often one of:

- **Point VoiceWatch at the camera's direct RTSP URL, not an ONVIF address.** ONVIF endpoints are not supported.
- Make sure the stream is reachable from inside the container, and prefer a mono audio stream.
- If your camera only exposes an awkward codec, transcode it to PCM with an intermediary like go2rtc or MediaMTX.

Restart VoiceWatch after changing a source to force a fresh stream check.

### After an update my RTSP detections dropped to zero or the audio sounds slowed down

This was a regression affecting streams below 48 kHz and stereo streams. Update to the latest nightly. As a tell, **Settings > Audio** shows roughly double the expected data rate (about 188 KB/s instead of about 94 KB/s at 48 kHz) when a stereo stream is being misread as mono. The lasting fix is to send a true mono stream from a single mic; you can also force mono at the source with `-ac 1` in your streamer's FFmpeg command.

### My RTSP password has a `#` (or other special character) and won't save

Percent-encode reserved characters in the password: `#` becomes `%23`, `@` becomes `%40`, `:` becomes `%3A`, `/` becomes `%2F`, `%` becomes `%25`. Example: `rtsp://user:pa%23ss@host:554/stream`.

### My RTSP stream won't connect on an older system (FFmpeg 4.x)

If a stream that works elsewhere fails with errors like "Unable to open RTSP for listening" or "Cannot assign requested address", you're likely on FFmpeg 4.x (common on a native install on Debian 12 / older distros). FFmpeg 4.x needs `-stimeout` where 5.x uses `-timeout`. The simplest fix is to use the **Docker install**, which ships a current FFmpeg; on a native install, upgrade FFmpeg to 5.x or newer (and check you don't have a stale FFmpeg left over from an old custom repo).

### How do I stream audio from a Raspberry Pi (or ESP32) into VoiceWatch?

The canonical recipe is FFmpeg plus MediaMTX on the Pi. Install FFmpeg, download the correct-architecture MediaMTX, and add a path that runs FFmpeg from your sound card to a local RTSP endpoint:

```yaml
paths:
  stream:
    runOnInit: ffmpeg -f alsa -i default -acodec libmp3lame -f rtsp -rtsp_transport tcp rtsp://localhost:8554/stream
    runOnInitRestart: yes
```

Point VoiceWatch at `rtsp://<pi-ip>:8554/stream`. Prefer PCM over MP3 for better analysis quality, and use `-i hw:1,0` (find devices with `arecord -l`) if `default` doesn't work. There are also community ESP32 RTSP-mic firmware projects listed in the README.

## Models and detection

See the [Detection Pipeline](detection-pipeline.md) for how audio flows through voice detection.

### Where did the Sensitivity and Overlap settings go?

They were intentionally removed from the UI. They are values that should be left at their defaults (sensitivity 1.0; overlap is adjusted automatically by the false-positive filter). They still exist in `config.yaml` if you really need them.

### The model download fails (e.g. Hugging Face is blocked here)

You can install a model manually. The exact names matter: the folder must be `perch-v2`, the model file `perch_v2.onnx`, and a `perch_v2_labels.txt` alongside it (rename Hugging Face's `labels.txt`). Place the folder under your models directory (logged at startup as `models_dir=...`). The mirror `hf-mirror.com` uses the same path layout.

## Improving accuracy and reducing false positives

### How do I cut down on false detections?

In rough order of impact:

1. **Enable the false-positive filter** (the "balanced" level is a good default). It requires a voice event to be detected several times within a short window before it is confirmed.
2. **Don't use aggressive EQ.** The VAD model was trained on unfiltered audio, so high-pass and band-reject filters distort the spectral signature and usually do more harm than good. The only safe uses are a narrow 50/60 Hz mains-hum notch or a gentle high-pass below ~200 Hz.

### Voice is not being detected even though someone is clearly speaking

For weak or near-zero detections on a local mic where confidence is uniformly low, the microphone or sound card is the most likely cause — check the microphone placement and gain before assuming a software problem. On a camera or low-bitrate RTSP stream, the audio codec may be cutting off frequencies needed by the VAD. Watch the live spectrogram: if the speech range (roughly 80 Hz–8 kHz) is blank or very faint, the stream is not carrying those frequencies cleanly.

## Performance and hardware

### CPU usage jumped after an update

A fix changed `birdnet.threads: 0` to mean "use all CPU cores" (it was previously single-threaded by accident). On many-core machines, or with BirdNET and Perch both loaded, that raises CPU use. Set an explicit thread count, e.g. `birdnet.threads: 4` in config or `BIRDNET_THREADS=4`. On a 2-core box use `1` to leave a core free. Perch is also simply heavier than BirdNET 2.4.

### Is a Raspberry Pi Zero 2 W enough?

It is marginal and RAM-limited; it cannot run the Perch or bat models, and is not recommended for new deployments. It works well as a dedicated RTSP mic feeding a Pi 4/5 that does the inference. If you do run it standalone, use Pi OS Lite, avoid RTSP/live streaming, and enable spectrogram pre-rendering. For sizing, run `birdnet-go benchmark` (rough BirdNET 2.4 throughput: Pi 4B ~6 inferences/s, Pi 5 ~11/s). See [Recommended Hardware](hardware.md).

## Database

### How do I move from SQLite to MySQL?

Keep your SQLite database in place, enable MySQL in config, and start VoiceWatch; it migrates your data into MySQL automatically and shows progress in the UI (it can take a while for large databases, let it finish). Do **not** pre-create tables with your own script, that confuses the startup detection. If a SQLite-to-MySQL switch gets wedged (MySQL ends up empty but the UI thinks migration completed), switch back to SQLite (your data is still there) and wait for the fix.

### Startup fails after an upgrade with a database/schema error

Use the [Database Doctor](https://github.com/tphakala/voicewatch/wiki/Database-Doctor) to diagnose and `--fix` (it backs up first). Don't drop tables yourself, several hold real data.

## Remote access, TLS, and authentication

### What's the recommended way to expose VoiceWatch to the internet?

Put it behind a reverse proxy (or a [Cloudflare Tunnel](cloudflare_tunnel_guide.md)) that terminates TLS, and point the proxy at VoiceWatch's plain HTTP port. Don't have VoiceWatch handle certificates itself when it is behind a proxy.

### Behind my own reverse proxy, audio won't play or the live spectrogram is blank

Two proxy settings cause this:

- **Audio playback / clip seeking** needs HTTP Range requests, which don't survive gzip. **Disable gzip for the audio endpoints** (`/api/v2/audio/...`) in your proxy (nginx/Caddy/Cloudflare).
- **The live spectrogram and live feed** use Server-Sent Events, which must not be buffered. In nginx set `proxy_buffering off;` and `X-Accel-Buffering: no` for those routes. Cloudflare's proxy can also interfere with long-lived SSE connections.

### I log in but get bounced back to the login page (over plain HTTP)

Set `security.redirecttohttps: false` and restart. With it enabled, session cookies are marked `Secure`, and browsers refuse to store those over plain `http://`, so the session is dropped. Only keep it enabled when you are actually behind HTTPS.

### Basic-auth / OIDC questions

- A basic-auth change that doesn't take effect: update to the latest nightly (auth now reads live config); on older builds, restart.
- OIDC "no provider for openid-connect exists" is almost always a wrong **Issuer URL**, it must be the URL that serves `.well-known/openid-configuration`.
- **Let's Encrypt / AutoTLS is currently broken** (it binds the wrong port and doesn't persist certificates). Until it is fixed, disable built-in TLS and terminate TLS at a reverse proxy instead.

### I locked myself out of authentication

Run the `reset_auth.sh` script from the repo against your `config.yaml`; it backs up the config and disables the auth blocks so you can get back in and reconfigure.

### How do I get rid of the `:8080` in the URL?

Put a reverse proxy in front (it can serve on port 80/443 and proxy to 8080). install.sh binds port 80 for AutoTLS renewal, so you can't simply move the web port to 80 without conflict.

## Integrations and notifications

### My Home Assistant MQTT sensors stay "Unknown"

MQTT discovery messages are retained by the broker, so after updating you need to republish them: **Settings > Integrations > MQTT > Send Discovery** (or restart VoiceWatch) once your streams are registered. MQTT TLS certificates (CA/client cert/key) are uploaded through that same MQTT settings page.

### Push notifications never arrive even though the test works

Update to the latest nightly (an old timeout bug made every push time out instantly). Also note VoiceWatch's two-layer model: the **provider** (URL/template) lives in config, and an **alert rule** decides when to send. Built-in rules only ring the in-app bell by default, so open the rule and add a **Push** action. Use the "Voice detected" event for every detection. For Discord, send a proper JSON embed template, not plain text lines.

### Can I embed the VoiceWatch UI in a Home Assistant card?

Enable `AllowEmbedding`. If buttons/saving fail when framed, also allow your Home Assistant origin via CORS (`http.cors_allow_origin`).

## Spectrograms, clips, and the web UI

### Audio clips won't play in Chrome (but work in Safari/Firefox)

This was a server compression bug, update to the latest nightly. It is not a Cloudflare problem.

### Audio and spectrograms 404 after an upgrade, or spectrograms never generate

Update to the latest nightly (there were path-validation and Windows-specific spectrogram fixes). Make sure clip recording is enabled (**Settings > Audio > Recording**) and the format is WAV or FLAC. If old clips are 404ing, they may be in a `data/2026/` folder instead of `data/clips/2026/`, move them under `clips/`.

### Dashboard search returns unexpected results

Update to the latest nightly and make sure you've upgraded to the v2 database schema (**System > Database**, the upgrade option only shows if you're still on the legacy schema).

### The setup wizard keeps reappearing

Update to the latest nightly. If it still loops, your `config.yaml` is likely corrupted, the UI falls back to the wizard when it can't read settings. Delete the corrupted `config.yaml` and re-run setup.

### Detections disappeared from my dashboard

The main dashboard shows a limited number of recent detections and is filtered to the current day. Increase the limit in the dashboard settings, or use the Search page with the period set to "All".

## Diagnostics and getting help

### How do I get help with a problem?

Generate a support dump from **Settings > Support** (it bundles your config and logs, and uploads independently of the telemetry setting). Reference it in a [discussion](https://github.com/tphakala/voicewatch/discussions) or [issue](https://github.com/tphakala/voicewatch/issues). If the upload times out on a Pi (large logs), stop your audio streams first or rotate the log, then try again, it often completes in the background despite the UI error.

### Can VoiceWatch batch-analyze my existing audio files?

No, it is built for real-time stream and sound-card analysis. VoiceWatch processes audio in real time only; offline file analysis is not currently supported.

### How do I back up my data?

Use the SQLite backup button under **System > Database**, or simply copy your data directory while running (SQLite's WAL mode allows consistent reads). For install.sh setups that's `~/birdnet-go-app/data/`, back up the `.db` files plus `config.yaml`. Scheduled/automated backups are planned but not yet available.
