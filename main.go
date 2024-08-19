package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/cdzombak/heartbeat"
	"github.com/influxdata/influxdb-client-go/v2"
)

var version = "<dev>"

func readNut(ups, key string) (string, error) {
	nutCmd := exec.Command("upsc", ups, key)
	nutOut, err := nutCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %s", key, err)
	}
	retv := strings.TrimSpace(string(nutOut))
	return retv, nil
}

func readNutInt(ups, key string) (int, error) {
	nutVal, err := readNut(ups, key)
	if err != nil {
		return 0, err
	}
	retv, err := strconv.Atoi(nutVal)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s '%s' to int: %s", key, nutVal, err)
	}
	return retv, nil
}

func readNutFloat(ups, key string) (float64, error) {
	nutVal, err := readNut(ups, key)
	if err != nil {
		return 0, err
	}
	retv, err := strconv.ParseFloat(nutVal, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s '%s' to float64: %s", key, nutVal, err)
	}
	return retv, nil
}

func main() {
	var influxServer = flag.String("influx-server", "", "InfluxDB server, including protocol and port, eg. 'http://192.168.1.1:8086'. Required.")
	var influxUser = flag.String("influx-username", "", "InfluxDB username.")
	var influxPass = flag.String("influx-password", "", "InfluxDB password.")
	var influxBucket = flag.String("influx-bucket", "", "InfluxDB bucket. Supply a string in the form 'database/retention-policy'. For the default retention policy, pass just a database name (without the slash character). Required.")
	var measurementName = flag.String("measurement-name", "ups_stats", "InfluxDB measurement name.")
	var upsNameTag = flag.String("ups-nametag", "", "Value for the ups_name tag in InfluxDB. Required.")
	var ups = flag.String("ups", "", "UPS to read status from, format 'upsname[@hostname[:port]]'. Required.")
	var pollInterval = flag.Int("poll-interval", 30, "Polling interval, in seconds.")
	var printUsage = flag.Bool("print-usage", false, "Log energy usage (in watts) to standard error.")
	var influxTimeoutS = flag.Int("influx-timeout", 3, "Timeout for writing to InfluxDB, in seconds.")
	var heartbeatURL = flag.String("heartbeat-url", "", "URL to GET every 60s, if and only if the program has successfully sent NUT statistics to Influx in the past 120s.")
	var printVersion = flag.Bool("version", false, "Print version and exit.")
	flag.Parse()

	if *printVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if *influxServer == "" || *influxBucket == "" {
		fmt.Println("-influx-bucket and -influx-server must be supplied.")
		os.Exit(1)
	}
	if *upsNameTag == "" || *ups == "" {
		fmt.Println("-ups and -ups-nametag must be supplied.")
		os.Exit(1)
	}

	var hb heartbeat.Heartbeat
	var err error
	if *heartbeatURL != "" {
		hb, err = heartbeat.NewHeartbeat(&heartbeat.Config{
			HeartbeatInterval: 60 * time.Second,
			LivenessThreshold: 120 * time.Second,
			HeartbeatURL:      *heartbeatURL,
			OnError: func(err error) {
				log.Printf("heartbeat error: %s\n", err)
			},
		})
		if err != nil {
			log.Fatalf("failed to create heartbeat client: %v", err)
		}
	}

	influxTimeout := time.Duration(*influxTimeoutS) * time.Second
	authString := ""
	if *influxUser != "" || *influxPass != "" {
		authString = fmt.Sprintf("%s:%s", *influxUser, *influxPass)
	}
	influxClient := influxdb2.NewClient(*influxServer, authString)
	ctx, cancel := context.WithTimeout(context.Background(), influxTimeout)
	defer cancel()
	health, err := influxClient.Health(ctx)
	if err != nil {
		log.Fatalf("failed to check InfluxDB health: %v", err)
	}
	if health.Status != "pass" {
		log.Fatalf("InfluxDB did not pass health check: status %s; message '%s'", health.Status, *health.Message)
	}
	influxWriteAPI := influxClient.WriteAPIBlocking("", *influxBucket)

	doUpdate := func() {
		atTime := time.Now()

		battCharge, err := readNutInt(*ups, "battery.charge")
		if err != nil {
			log.Println(err.Error())
			return
		}
		battChargeLow, err := readNutInt(*ups, "battery.charge.low")
		if err != nil {
			log.Println(err.Error())
			return
		}
		battRuntime, err := readNutInt(*ups, "battery.runtime")
		if err != nil {
			log.Println(err.Error())
			return
		}
		battV, err := readNutFloat(*ups, "battery.voltage")
		if err != nil {
			log.Println(err.Error())
			return
		}
		battVNominal, err := readNutFloat(*ups, "battery.voltage.nominal")
		if err != nil {
			log.Println(err.Error())
			return
		}
		inputV, err := readNutFloat(*ups, "input.voltage")
		if err != nil {
			log.Println(err.Error())
			return
		}
		inputVNominal, err := readNutFloat(*ups, "input.voltage.nominal")
		if err != nil {
			log.Println(err.Error())
			return
		}

		load, err := readNutInt(*ups, "ups.load")
		if err != nil {
			log.Println(err.Error())
			return
		}

		nominalPower, err := readNutInt(*ups, "ups.realpower.nominal")
		if err != nil {
			var err2 error
			nominalPower, err2 = readNutInt(*ups, "ups.power.nominal")
			if err2 != nil {
				log.Println(err.Error())
				log.Println(err2.Error())
				return
			}
		}

		var power float64
		if power, err = readNutFloat(*ups, "ups.power"); err == nil {
			if *printUsage {
				log.Printf("current output for '%s': %.f watts\n", *ups, power)
			}
		} else {
			power = math.Round(float64(nominalPower) * float64(load) / 100.0)
			if *printUsage {
				log.Printf("current approx. output for '%s': %.f watts\n", *ups, power)
			}
		}

		fields := map[string]interface{}{
			"watts":                      power, // backward compatibility
			"power":                      power,
			"power_nominal":              nominalPower,
			"load_percent":               load,
			"battery_charge_percent":     battCharge,
			"battery_charge_low_percent": battChargeLow,
			"battery_runtime_s":          battRuntime,
			"battery_voltage":            battV,
			"battery_voltage_nominal":    battVNominal,
			"input_voltage":              inputV,
			"input_voltage_nominal":      inputVNominal,
		}

		// optional properties follow:

		if outputV, err := readNutFloat(*ups, "output.voltage"); err == nil {
			fields["output_voltage"] = outputV
		} else {
			log.Println(err.Error())
		}
		if outputVNominal, err := readNutFloat(*ups, "output.voltage.nominal"); err == nil {
			fields["output_voltage_nominal"] = outputVNominal
		} else {
			log.Println(err.Error())
		}
		if outputCurrent, err := readNutFloat(*ups, "output.current"); err == nil {
			fields["output_current"] = outputCurrent
		} else {
			log.Println(err.Error())
		}
		if battChargeWarning, err := readNutInt(*ups, "battery.charge.warning"); err == nil {
			fields["battery_charge_warning_percent"] = battChargeWarning
		} else {
			log.Println(err.Error())
		}
		if battTemp, err := readNutFloat(*ups, "battery.temperature"); err == nil {
			fields["battery_temperature_c"] = battTemp
			fields["battery_temperature_f"] = math.Round(battTemp*9.0/5.0 + 32.0)
		} else {
			log.Println(err.Error())
		}
		if inputFreq, err := readNutFloat(*ups, "input.frequency"); err == nil {
			fields["input_frequency"] = inputFreq
		} else {
			log.Println(err.Error())
		}
		if outputFreq, err := readNutFloat(*ups, "output.frequency"); err == nil {
			fields["output_frequency"] = outputFreq
		} else {
			log.Println(err.Error())
		}
		if outputFreqNominal, err := readNutFloat(*ups, "output.frequency.nominal"); err == nil {
			fields["output_frequency_nominal"] = outputFreqNominal
		} else {
			log.Println(err.Error())
		}

		point := influxdb2.NewPoint(
			*measurementName,
			map[string]string{"ups_name": *upsNameTag}, // tags
			fields,
			atTime,
		)
		if err := retry.Do(
			func() error {
				ctx, cancel := context.WithTimeout(context.Background(), influxTimeout)
				defer cancel()
				return influxWriteAPI.WritePoint(ctx, point)
			},
			retry.Attempts(2),
		); err != nil {
			log.Printf("failed to write point to influx: %v", err)
		} else if hb != nil {
			hb.Alive(atTime)
		}
	}

	if hb != nil {
		hb.Start()
	}
	doUpdate()
	for range time.Tick(time.Duration(*pollInterval) * time.Second) {
		doUpdate()
	}
}
