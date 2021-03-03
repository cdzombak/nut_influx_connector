
.PHONY: all
all:
	rm -rf ./out
	mkdir ./out
	env GOOS=linux GOARCH=arm GOARM=6 go build -o out/nut_influx_connector_linux_armv6 .
	env GOOS=linux GOARCH=arm GOARM=7 go build -o out/nut_influx_connector_linux_armv7 .
	env GOOS=linux GOARCH=amd64 go build -o out/nut_influx_connector_linux_amd64 .
