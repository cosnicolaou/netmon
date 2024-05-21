package main

import (
	"context"
	"os"

	"cloudeng.io/cmdutil"
	"cloudeng.io/cmdutil/subcmd"
	"cloudeng.io/errors"
)

const cmdSpec = `name: netmon
summary: device and network monitoring tool
commands:
  - name: devices
    summary: manage devices
    commands:
      - name: monitor  
        summary: monitor devices according to the specified configuration files
        args:
          - <device>... - the devices to monitor, monitor all if none specified
`

func cli() *subcmd.CommandSetYAML {
	cmd := subcmd.MustFromYAML(cmdSpec)
	dev := &Devices{}
	cmd.Set("devices", "monitor").MustRunner(dev.Monitor, &DeviceMonitorFlags{})
	return cmd
}

var interrupt = errors.New("interrupt")

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancelCause(ctx)
	cmdutil.HandleSignals(func() { cancel(interrupt) }, os.Interrupt)
	err := cli().Dispatch(ctx)
	if context.Cause(ctx) == interrupt {
		cmdutil.Exit("%v", interrupt)
	}
	cmdutil.Exit("%v", err)
}
