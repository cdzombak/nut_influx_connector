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
	"github.com/influxdata/influxdb-client-go/v2"
)

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
	flag.Parse()
	if *influxServer == "" || *influxBucket == "" {
		fmt.Println("-influx-bucket and -influx-server must be supplied.")
		os.Exit(1)
	}
	if *upsNameTag == "" || *ups == "" {
		fmt.Println("-ups and -ups-nametag must be supplied.")
		os.Exit(1)
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
	influxWriteApi := influxClient.WriteAPIBlocking("", *influxBucket)

	doUpdate := func() {
		atTime := time.Now()

		load, err := readNutInt(*ups, "ups.load")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		nominalPower, err := readNutInt(*ups, "ups.realpower.nominal")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		battCharge, err := readNutInt(*ups, "battery.charge")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		battChargeLow, err := readNutInt(*ups, "battery.charge.low")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		battRuntime, err := readNutInt(*ups, "battery.runtime")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		battV, err := readNutFloat(*ups, "battery.voltage")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		battVNominal, err := readNutFloat(*ups, "battery.voltage.nominal")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		inputV, err := readNutFloat(*ups, "input.voltage")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		inputVNominal, err := readNutFloat(*ups, "input.voltage.nominal")
		if err != nil {
			log.Printf(err.Error())
			return
		}
		outputV, err := readNutFloat(*ups, "output.voltage")
		if err != nil {
			log.Printf(err.Error())
		}

		watts := math.Round(float64(nominalPower) * float64(load) / 100.0)
		if *printUsage {
			log.Printf("current approx. output for '%s': %.f watts", *ups, watts)
		}

		point := influxdb2.NewPoint(
			*measurementName,
			map[string]string{"ups_name": *upsNameTag}, // tags
			map[string]interface{}{ // fields
				"watts":                      watts,
				"load_percent":               load,
				"battery_charge_percent":     battCharge,
				"battery_charge_low_percent": battChargeLow,
				"battery_runtime_s":          battRuntime,
				"battery_voltage":            battV,
				"battery_voltage_nominal":    battVNominal,
				"input_voltage":              inputV,
				"input_voltage_nominal":      inputVNominal,
				"output_voltage":             outputV,
			},
			atTime,
		)
		if err := retry.Do(
			func() error {
				ctx, cancel := context.WithTimeout(context.Background(), influxTimeout)
				defer cancel()
				return influxWriteApi.WritePoint(ctx, point)
			},
			retry.Attempts(2),
		); err != nil {
			log.Printf("failed to write point to influx: %v", err)
		}
	}

	doUpdate()
	for {
		select {
		case <-time.Tick(time.Duration(*pollInterval) * time.Second):
			doUpdate()
		}
	}
}
