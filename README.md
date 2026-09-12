# dorm-gateway

A lightweight, fast CLI for authenticating to GSB dormitory WiFi captive portals with optional Cloudflare WARP integration.

[![macOS](https://img.shields.io/badge/macOS-000000?style=flat-square&logo=apple&logoColor=white)](#installation)
[![Linux](https://img.shields.io/badge/Linux-FCC624?style=flat-square&logo=linux&logoColor=black)](#installation)
[![License: MIT](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)

**English** | [Türkçe](README_TR.md)

---

## Installation

### Prebuilt Binaries
Download the latest binary for your operating system and architecture from [Releases](https://github.com/dumbovita/dorm-gateway/releases), make it executable, and move it to your `PATH`:

```bash
chmod +x dorm-gateway
sudo mv dorm-gateway /usr/local/bin/
```

### Build from Source
Requires [Go 1.22+](https://go.dev/):

```bash
git clone https://github.com/dumbovita/dorm-gateway.git
cd dorm-gateway
make build
sudo mv bin/dorm-gateway /usr/local/bin/
```

---

## Configuration

Save your credentials once with restrictive permissions (`0600`):

```bash
# Secure interactive prompt (password input is hidden)
dorm-gateway config set
```

Or configure via environment variables:

```bash
export DORM_GATEWAY_USERNAME="12345678901"
export DORM_GATEWAY_PASSWORD="yourPassword"
```

To view current settings or locate the config file:
```bash
dorm-gateway config show
dorm-gateway config path
```

---

## Usage

### Authenticate
Log into the captive portal:
```bash
dorm-gateway auth
```

### Connect Everything (`up`)
Authenticate to the portal and connect Cloudflare WARP in one step:
```bash
dorm-gateway up
```

### Check Status
View portal authentication, internet connectivity, and WARP status:
```bash
dorm-gateway status
```

### Run Diagnostics
Inspect DNS, captive portal reachability, config permissions, and WARP state:
```bash
dorm-gateway doctor
```

---

## Cloudflare WARP (Optional)

`dorm-gateway` integrates with official Cloudflare WARP (`warp-cli`) to provide an encrypted privacy tunnel after captive portal login:

```bash
# Connect WARP
dorm-gateway warp up

# Disconnect WARP
dorm-gateway warp down

# Inspect WARP tunnel status
dorm-gateway warp status
```

If `warp-cli` is not installed on your system, install the official package for macOS ([1.1.1.1](https://1.1.1.1)) or Linux ([pkg.cloudflareclient.com](https://pkg.cloudflareclient.com/)).

---

## Troubleshooting

- **Portal timeout or congestion:** Captive portals often experience high traffic during peak hours. `dorm-gateway auth` automatically retries with exponential backoff and jitter.
- **WARP connected but no traffic:** Check if your dorm network blocks UDP. You can switch to the MASQUE protocol:
  ```bash
  dorm-gateway warp up --protocol masque
  ```
- **System checks:** Run `dorm-gateway doctor` to verify local DNS resolution, internet reachability, and WARP daemon health.

---

## Disclaimer

This software is provided for educational and informational purposes only, on an "as-is" basis without warranties of any kind. Use of this software is entirely at your own risk. Users are solely responsible for complying with all applicable laws, institutional regulations, network policies, and terms of service. To the extent permitted by applicable law, the maintainers and contributors assume no liability for account suspensions, network access restrictions, service disruptions, data loss, disciplinary actions, or any other direct or indirect consequences arising from the use or misuse of this tool.

---

## Credits

- **Inspiration & Concept:** Deniz Egemen Emare ([@denizZz009](https://github.com/denizZz009))

---

## License

Distributed under the [MIT License](LICENSE).

---

⭐ If you find this tool helpful, please consider leaving a star on GitHub!
