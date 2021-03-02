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

func main() {
	var influxServer = flag.String("influx-server", "", "InfluxDB server, including protocol and port, eg. 'http://192.168.1.1:8086'. Required.")
	var influxUser = flag.String("influx-username", "", "InfluxDB username.")
	var influxPass = flag.String("influx-password", "", "InfluxDB password.")
	var influxBucket = flag.String("influx-bucket", "", "InfluxDB bucket. Supply a string in the form 'database/retention-policy'. For the default retention policy, pass just a database name (without the slash character). Required.")
	var upsNameTag = flag.String("ups-nametag", "", "Value for the ups_name tag in InfluxDB. Required.")
	var ups = flag.String("ups", "", "UPS to read status from, format 'upsname[@hostname[:port]]'. Required.")
	var pollInterval = flag.Int("poll-interval", 30, "Polling interval, in seconds.")
	var printUsage = flag.Bool("print-usage", false, "Log energy usage to standard error.")
	flag.Parse()
	if *influxServer == "" || *influxBucket == "" {
		fmt.Println("-influx-bucket and -influx-server must be supplied.")
		os.Exit(1)
	}
	if *upsNameTag == "" || *ups == "" {
		fmt.Println("-ups and -ups-nametag must be supplied.")
		os.Exit(1)
	}

	const influxTimeout = 3 * time.Second
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

		loadCmd := exec.Command("upsc", *ups, "ups.load")
		loadOut, err := loadCmd.Output()
		if err != nil {
			log.Printf("failed to read ups.load: %s", err)
			return
		}
		load, err := strconv.Atoi(strings.TrimSpace(string(loadOut)))
		if err != nil {
			log.Printf("failed to parse ups.load '%s' to int: %s", loadOut, err)
			return
		}

		nominalPowerCmd := exec.Command("upsc", *ups, "ups.realpower.nominal")
		nominalPowerOut, err := nominalPowerCmd.Output()
		if err != nil {
			log.Printf("failed to read ups.realpower.nominal: %s", err)
			return
		}
		nominalPower, err := strconv.Atoi(strings.TrimSpace(string(nominalPowerOut)))
		if err != nil {
			log.Printf("failed to parse ups.realpower.nominal '%s' to int: %s", nominalPowerOut, err)
			return
		}

		watts := math.Round(float64(nominalPower) * float64(load) / 100.0)

		if *printUsage {
			log.Printf("current approx. output for '%s': %.f watts", *ups, watts)
		}

		point := influxdb2.NewPoint(
			"ups_power_output",
			map[string]string{"ups_name": *upsNameTag}, // tags
			map[string]interface{}{"watts": watts},     // fields
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
