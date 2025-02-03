# FFGestures 🖐️

Multi-touch gesture tool for FreeBSD touchscreens, tested only on Panasonic FZ-G1 tablet. Supports 1-10 finger gestures.

## ✨ Features
- 🖐️ Multi-finger detection (Up to 10 figures)
- ⬅️➡️⬆️⬇️ Swipe direction recognition
- ⚙️ JSON-configurable actions
- Uses raw libinput command with no other dependencies

*Tested on FreeBSD running on Panasonic FZ-G1 tablets.*

## 🚀 Quick Start

### Set up permissions

```bash
# Setup input group and permissions
pw groupadd input
pw usermod [USERNAME] -G input

# Configure devfs rules
cat << EOF >> /etc/devfs.rules
[inputrules=10]
add path 'input/event*' mode 0660 group input
EOF
sysrc devfs_system_ruleset="inputrules"
```

### Install and run

```bash
# Install libinput first
pkg install -y libinput

# Build and run
go build -o ffgestures main.go
./ffgestures -c config.json
```

## ⚙️ Configuration Example

Create `config.json`:

```json
{
  "threshold": 10.0,
  "gestureActions": {
    "1swipe_left": "echo '1-finger swipe left action executed'",
    "1swipe_right": "echo '1-finger swipe right action executed'",
    "1swipe_up": "echo '1-finger swipe up action executed'",
    "1swipe_down": "echo '1-finger swipe down action executed'",
    "3swipe_left": "echo '3-finger swipe left action executed'",
    "3swipe_right": "echo '3-finger swipe right action executed'",
    "3swipe_up": "pgrep -f wvkbd-mobintl || wvkbd-mobintl -L 500 --fn \"Sans Bold 24\" --text ffbf40 --text-sp ffbf40 --bg 000000 --fg 222222 --fg-sp 333333 --press 444444 --press-sp 444444",
    "3swipe_down": "pkill -f wvkbd-mobintl",
    "2swipe_up": "pgrep -f wvkbd-mobintl || wvkbd-mobintl -L 500 --fn \"Sans Bold 24\" --text ffbf40 --text-sp ffbf40 --bg 000000 --fg 222222 --fg-sp 333333 --press 444444 --press-sp 444444",
    "2swipe_down": "pkill -f wvkbd-mobintl"
  },
  "debug": false
}
```

## 📄 License

MIT License
