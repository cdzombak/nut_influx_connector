# nut_influx_connector

Ship energy usage data and other UPS stats from Network-UPS-Tools to InfluxDB.

The following fields are written to an InfluxDB measurement on a periodic basis:

- `battery_charge_low_percent`: "Low" battery charge percentage.
- `battery_charge_percent`: Battery charge percentage.
- `battery_runtime_s`: Battery runtime, in seconds.
- `battery_voltage`: Battery voltage.
- `battery_voltage_nominal`: Nominal battery voltage.
- `input_voltage`: Input voltage.
- `input_voltage_nominal`: Nominal input voltage.
- `load_percent`: Load percentage of the UPS.
- `power`: Current power output in watts.
- `power_nominal`: Nominal (maximum) power output in watts.
- `watts`: Current power output in watts (duplicates the value written to `power`, for backward compatibility).

The following fields _may_ be written, if NUT supports them for your UPS:

- `battery_charge_warning_percent`: "Warning" battery charge percentage.
- `battery_temperature_c`: Battery temperature, in Celsius.
- `battery_temperature_f`: Battery temperature, in Fahrenheit.
- `input_frequency`: Input frequency.
- `output_current`: Output current.
- `output_frequency`: Output frequency.
- `output_frequency_nominal`: Nominal output frequency.
- `output_voltage`: Output voltage.
- `output_voltage_nominal`: Nominal output voltage.

## Usage

The following syntax will run `nut_influx_connector`, and the program will keep running until it's killed.

### InfluxDB Output Example

```text
nut_influx_connector \
    -influx-bucket "nut/autogen" \
    -influx-server http://192.168.1.4:8086 \
    -ups deskups \
    -ups-nametag "work_desk" \
    [OPTIONS ...]
```

### MQTT Output Example

```text
nut_influx_connector \
    -mqtt-server tcp://192.168.1.1:1883 \
    -mqtt-topic "home/UPS" \
    -mqtt-username "mqtt_user" \
    -mqtt-password "mqtt_pass" \
    -ups deskups \
    -ups-nametag "work_desk" \
    [OPTIONS ...]
```

This will publish measurements to topics like `home/UPS/battery_charge_percent`, `home/UPS/power`, etc.

## Options

### General Options

- `-ups string`: UPS to read status from, format `upsname[@hostname[:port]]`. Required.
- `-ups-nametag string`: Value for the `ups_name` tag in InfluxDB. Required.
- `-poll-interval int`: Polling interval, in seconds. (default `30`)
- `-print-usage`: Log energy usage (in watts) to standard error.
- `-heartbeat-url string`: URL to GET every 60s, if and only if the program has successfully sent NUT statistics to output in the past 120s.
- `-help`: Print help and exit.
- `-version`: Print version and exit.

### InfluxDB Options

- `-influx-server string`: InfluxDB server, including protocol and port, e.g. `http://192.168.1.4:8086`.
- `-influx-bucket string`: InfluxDB bucket. Supply a string in the form `database/retention-policy`. For the default retention policy, pass just a database name (without the slash character).
- `-influx-username string`: InfluxDB username.
- `-influx-password string`: InfluxDB password.
- `-influx-timeout int`: Timeout for writing to InfluxDB, in seconds. (default `3`)
- `-measurement-name string`: InfluxDB measurement name. (default `ups_stats`)

### MQTT Options

- `-mqtt-server string`: MQTT broker server, including protocol and port, e.g. `tcp://192.168.1.1:1883`.
- `-mqtt-topic string`: MQTT base topic for publishing measurements.
- `-mqtt-username string`: MQTT username.
- `-mqtt-password string`: MQTT password.

**Note:** At least one output method (InfluxDB or MQTT) must be configured.

## Installation

### Debian via Apt repository

Install my Debian repository if you haven't already:

```shell
sudo apt install ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://dist.cdzombak.net/deb.key | sudo gpg --dearmor -o /etc/apt/keyrings/dist-cdzombak-net.gpg
sudo chmod 0644 /etc/apt/keyrings/dist-cdzombak-net.gpg
echo -e "deb [signed-by=/etc/apt/keyrings/dist-cdzombak-net.gpg] https://dist.cdzombak.net/deb/oss any oss\n" | sudo tee -a /etc/apt/sources.list.d/dist-cdzombak-net.list > /dev/null
sudo apt update
```

Then install `nut-influx-connector` via `apt`:

```shell
sudo apt install nut-influx-connector
```

### macOS via Homebrew

```shell
brew install cdzombak/oss/nut_influx_connector
```

### Manual installation from build artifacts

Pre-built binaries for Linux and macOS on various architectures are downloadable from each [GitHub Release](https://github.com/cdzombak/nut_influx_connector/releases). Debian packages for each release are available as well.

### Build and install locally

```shell
git clone https://github.com/cdzombak/nut_influx_connector.git
cd nut_influx_connector
make build

cp out/nut_influx_connector $INSTALL_DIR
```

## Running on Linux with Systemd

After installing the binary, you can run it as a systemd service.

- Optionally, create a user for the service to run as: `sudo useradd -r -s /usr/sbin/nologin nut-influx-connector`
- Install the systemd service `nut-influx-connector.service` and customize that file as desired (e.g. with the correct CLI options for your deployment):
```shell
curl -sSL https://raw.githubusercontent.com/cdzombak/nut_influx_connector/main/nut-influx-connector.service | sudo tee /etc/systemd/system/nut-influx-connector.service
sudo nano /etc/systemd/system/nut-influx-connector.service
```
- Enable and start the service:
```shell
sudo systemctl daemon-reload
sudo systemctl enable nut-influx-connector
sudo systemctl start nut-influx-connector
```
- Verify its operation:
```shell
sudo systemctl status nut-influx-connector
sudo journalctl -f -u nut-influx-connector.service
```

## Running on macOS with Launchd

After installing the binary via Homebrew, you can run it as a launchd service.
- Install the launchd plist `com.dzombak.nut-influx-connector.plist` and customize that file as desired (e.g. with the correct CLI options for your deployment):
```shell
mkdir -p "$HOME"/Library/LaunchAgents
curl -sSL https://raw.githubusercontent.com/cdzombak/nut_influx_connector/main/com.dzombak.nut-influx-connector.plist > "$HOME"/Library/LaunchAgents/com.dzombak.nut-influx-connector.plist
nano "$HOME"/Library/LaunchAgents/com.dzombak.nut-influx-connector.plist
```

## Home Assistant Integration

When using MQTT output, you can integrate the UPS data with Home Assistant using MQTT sensors. Add the following to your Home Assistant configuration:

```yaml
mqtt:
  sensor:
    - name: "UPS Battery Charge"
      state_topic: "home/UPS/battery_charge_percent"
      unit_of_measurement: "%"
      device_class: "battery"
      
    - name: "UPS Power Output"
      state_topic: "home/UPS/power"
      unit_of_measurement: "W"
      device_class: "power"
      
    - name: "UPS Load"
      state_topic: "home/UPS/load_percent"
      unit_of_measurement: "%"
      
    - name: "UPS Input Voltage"
      state_topic: "home/UPS/input_voltage"
      unit_of_measurement: "V"
      device_class: "voltage"
      
    - name: "UPS Battery Runtime"
      state_topic: "home/UPS/battery_runtime_s"
      unit_of_measurement: "s"
```

Replace `home/UPS` with your configured MQTT topic.

## See Also

- [macos-ups-influx-connector](https://github.com/cdzombak/macos-ups-influx-connector)

## License

MIT; see `LICENSE` in this repository.

## Author

[Chris Dzombak](https://www.dzombak.com) (GitHub [@cdzombak](https://github.com/cdzombak)).
