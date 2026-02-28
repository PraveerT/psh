# PhoneSSH — Setup Guide

## Architecture

```
┌─────────────┐       JSON over TCP (port 8765)       ┌──────────────────┐
│  psh  (CLI) │ ◄──────────────────────────────────►  │  PhoneSSH (app)  │
│  (laptop)   │       via LAN or Tailscale VPN         │  (Android)       │
└─────────────┘                                        └──────────────────┘
```

Transport is **unencrypted TCP** — use Tailscale or a local network.
Auth is a **256-bit random token** (shown as QR code on the phone).

---

## Step 1 — Build and install the CLI

**Prerequisites:** Go 1.21+

```bash
# Clone and build
git clone <this-repo> phonessh
cd phonessh
make cli

# Or install to PATH
make install
```

Verify:
```bash
psh --help
```

---

## Step 2 — Install the Android app

**Prerequisites:** Android Studio, Android SDK, device with Android 11+ (API 31)

```bash
# Option A: Build and install directly to USB-connected phone
make android-install

# Option B: Build APK manually
cd android
./gradlew assembleDebug
# APK is at android/app/build/outputs/apk/debug/app-debug.apk
```

Or open `android/` in Android Studio and run from there.

---

## Step 3 — Grant permissions on your phone

Open the **PhoneSSH** app and grant the following:

| Permission | Used for | Required |
|---|---|---|
| Storage | `ls`, `pull`, `push` | Yes |
| Location | `psh location` | Optional |
| SMS | `psh sms` | Optional |
| Notification access | `psh notifs` | Optional |
| DND access | `psh dnd` | Optional |
| Write settings | `psh brightness` | Optional |

Tap each **Grant** button in the app for special permissions (notification access, DND, write settings).

---

## Step 4 — Pair your laptop

1. In the PhoneSSH app, tap **Start Service**
2. The QR code shows your pairing URL
3. On your laptop, run:

```bash
psh pair
# Paste the URL shown on your phone screen
```

Or if you can read the QR:
```bash
psh pair "psh://pair?host=192.168.1.42&port=8765&token=<token>&name=MyPhone"
```

---

## Step 5 — Use it!

```bash
# System info
psh status
psh battery
psh location

# Files
psh ls /sdcard/DCIM
psh pull /sdcard/DCIM/photo.jpg ./
psh push ./report.pdf /sdcard/Documents/
psh find "*.pdf" /sdcard/

# Notifications
psh notifs
psh notifs --clear slack
psh notifs --app gmail

# Messaging
psh sms list --unread
psh sms send "+1234567890" "Running late"

# Apps
psh apps list
psh apps launch spotify
psh apps kill twitter
psh apps info com.spotify.music

# System controls
psh volume set 50
psh brightness set 80
psh dnd on
psh dnd off
psh wifi status
psh clipboard get
psh clipboard set "hello from laptop"
psh screenshot
```

---

## Connectivity

### Local network (simplest)
Phone and laptop on same WiFi. Use the phone's local IP (shown in the app).

### Tailscale (recommended — works anywhere)
1. Install [Tailscale](https://tailscale.com) on both phone and laptop
2. Use the phone's Tailscale IP (`100.x.x.x`) when pairing
3. Works across different networks, behind NAT, on mobile data

### WireGuard
Set up a WireGuard tunnel with your phone's endpoint — pair using the WireGuard IP.

---

## Security notes

- Token is 256-bit random — cryptographically strong
- Traffic is **not encrypted** on the TCP level — use Tailscale/WireGuard which encrypts at the network level
- Rotate the token anytime via the app → **Rotate Token** button
- All commands are logged in the app's activity log
- Sensitive permissions (SMS, location) require explicit Android permission grants

---

## Troubleshooting

**`psh: cannot reach phone`**
- Is PhoneSSH running? Check the notification bar
- Same network? Try `ping <phone-ip>`
- Tailscale connected on both devices?

**`authentication failed`**
- Token may be rotated — re-run `psh pair`

**`notification access not enabled`**
- Open Settings > Apps > Special app access > Notification access > enable PhoneSSH

**`location permission not granted`**
- Open Settings > Apps > PhoneSSH > Permissions > Location > Allow

**`WRITE_SETTINGS needed`**
- Open Settings > Apps > PhoneSSH > Modify system settings > Allow

---

## Roadmap

- [ ] **Phase 2**: Ed25519 key-pair auth, chunked file transfer, audit log file, `psh run <script>`
- [ ] **Phase 3**: Natural language commands (`psh "clear slack and set DND for 1 hour"`)
- [ ] **Phase 4**: Plugin system, iOS companion via Shortcuts
